package qr2

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"math/rand"

	"wwfc/common"
	"wwfc/logging"

	"github.com/logrusorgru/aurora/v3"
	"github.com/sasha-s/go-deadlock"
)

var ServerName = "roommanager"
var rooms = map[string]*Room{}

var (
	connBuffers = map[uint64]*[]byte{}
	mutexRM     = deadlock.RWMutex{}
)

func NewConnection(index uint64, address string) {
	mutexRM.Lock()
	connBuffers[index] = &[]byte{}
	mutexRM.Unlock()
}

func CloseConnection(index uint64) {
	mutexRM.Lock()
	delete(connBuffers, index)
	mutexRM.Unlock()
}

func HandlePacket(index uint64, data []byte, address string) {
	mutexRM.RLock()
	buffer := connBuffers[index]
	mutexRM.RUnlock()

	if buffer == nil {
		buffer = &[]byte{}
		defer func() {
			if buffer == nil {
				return
			}

			mutexRM.Lock()
			connBuffers[index] = buffer
			mutexRM.Unlock()
		}()
	}

	*buffer = append(*buffer, data...)

	if len(*buffer)+len(data) > 0x1000 {
		logging.Error(moduleName, "Buffer overflow")
		// TODO: Probably try to check if the player is in a room and do any clean up there
		common.CloseConnection(ServerName, index)
		buffer = nil
		return
	}

	// Try to process as many packets as possible
	for len(*buffer) >= MatchRequestHeaderSize {
		matchRequestHeader, err := tryParseMatchRequestHeader((*buffer)[:MatchRequestHeaderSize])
		if err != nil {
			// The only way header parsing can fail is if the magic is bad (currently). Given that,
			// we only need to clear the buffer until the next valid magic. This is required
			// for subsequent parsing to work.
			next := bytes.Index(*buffer, []byte(MatchRequestHeaderMagic))
			if next == -1 {
				// Clear the buffer if valid magic wasn't found
				next = len(*buffer)
			}
			logging.Info(moduleName, "Failed to parse match request header from", address, "error is", err.Error())

			*buffer = (*buffer)[next:]
			continue
		}

		p, err := validateBasics(matchRequestHeader)
		if err != nil {
			logging.Info(moduleName, "Player failed basic validation for match request from", address, "error was", err.Error())
			common.CloseConnection(ServerName, index)
			buffer = nil
			return
		}

		// store connection information needed to send messages back to the player
		p.setRoomManagerConnection(index, address)

		requestType := matchRequestHeader.requestType

		// get the expected packet length for each message type
		var msgLen int
		switch requestType {
		case OpenRoom, LeaveRoom, OpenMogi:
			msgLen = 0x10
		case JoinFriend, Suspend, SearchPublicRoom, LocalPlayerCount, LeaderboardPage, ReplayList, MogiList, RankSetting:
			msgLen = 0x18
		default:
			logging.Info(moduleName, "Unknown request type sent by player", p.PlayerId, "type:", requestType)
			*buffer = (*buffer)[:0]
		}

		// data got cut off or message is invalid, break and wait until we recv again
		if len(*buffer) < msgLen {
			break
		}

		// we can process a complete message, update buffer and process the message
		msg := (*buffer)[:msgLen]
		*buffer = (*buffer)[msgLen:]

		switch requestType {
		case OpenRoom:
			logging.Info(moduleName, "Received OpenRoom request from player", p)
			err := handleOpenRoomRequest(p, false)
			if err != nil {
				logging.Info(moduleName, err.Error())
			}

		case OpenMogi:
			logging.Info(moduleName, "Received OpenMogi request from player", p)
			if err := handleOpenRoomRequest(p, true); err != nil {
				logging.Info(moduleName, err.Error())
			}

		case JoinFriend:
			logging.Info(moduleName, "Received JoinFriend request from", p.PlayerId)

			req := &JoinFriendRequest{
				header:          *matchRequestHeader,
				friendProfileId: binary.BigEndian.Uint32(msg[0x10:0x14]),
				searchRegion:    common.MKWServerSearchRegion(msg[0x14]),
			}

			err := handleJoinFriendRequest(p, req)
			if err != nil {
				logging.Info(moduleName, "JoinFriend failed for player %d and profileId %d with Reason:", err.Error())
			}
		case LeaveRoom:
			logging.Info(moduleName, "Received LeaveRoom request from", p.PlayerId)
			err := handleLeaveRoomRequest(p)
			if err != nil {
				logging.Info(moduleName, "LeaveRoom failed for reason:", err.Error())
			}

		case Suspend:
			suspendRequest := msg[0x10] != 0
			err := handleSuspendRequest(p, suspendRequest)
			if err != nil {
				logging.Info(moduleName, "Suspend failed for reason:", err.Error())
			}

		case SearchPublicRoom:
			searchReq := &SearchPublicRoomRequest{
				header: *matchRequestHeader,
				region: common.MKWServerSearchRegion(msg[0x10]),
				mode:   common.MKWServerGameMode(msg[0x11]),
				replay: int(msg[0x12]),
			}
			p.replayChoice = searchReq.replay
			logging.Info(moduleName, "Received SearchPublicRoom from player", p.PlayerId, "where region is", searchReq.region, "and mode is", searchReq.mode)
			err := handleSearchPublicRoomRequest(p, searchReq)
			if err != nil {
				logging.Info(moduleName, "SearchPublicRoom failed with reason:", err.Error())
			}

		case RankSetting:
			p.showLoungeRank = msg[0x10] != 0
			if r := p.roomPointer; r != nil {
				r.broadcastLoungeRanks()
			}

		case MogiList:
			if err := handleMogiListRequest(p); err != nil {
				logging.Info(moduleName, "Mogi list failed:", err.Error())
			}

		case ReplayList:
			if err := handleReplayListRequest(p); err != nil {
				logging.Info(moduleName, "Replay list failed:", err.Error())
			}

		case LeaderboardPage:
			if err := handleLeaderboardRequest(p, int(binary.BigEndian.Uint16(msg[0x10:0x12]))); err != nil {
				logging.Info(moduleName, "Leaderboard page failed:", err.Error())
			}

		case LocalPlayerCount:
			// Simple packet structure, just get the localPlayerCount from offset 0x10
			localPlayerCount := msg[0x10]

			logging.Info(moduleName, "Received LocalPlayerCount from", p.PlayerId, "where localPlayers is", localPlayerCount)

			err := handleSetLocalPlayerCount(p, localPlayerCount)
			if err != nil {
				logging.Info(moduleName, err.Error())
			}

		default:
			logging.Error(moduleName, "Unknown request type", aurora.Cyan(requestType))
		}
	}
}

// validates basic requirements to even make a match making request
// more can be added here
func validateBasics(header *MatchRequestHeader) (*Player, error) {
	// convert to int so we can check if the player exists
	p, _ := playerBySearchID[header.searchId]
	if p == nil {
		return nil, fmt.Errorf("No player with searchId %d exists", header.searchId)

	}

	// validate as much as we can, check for Authenticated, ExploitReceived, roomPointer == nil, etc
	if !p.Authenticated {
		return nil, fmt.Errorf("PlayerId %d is not authenticated", p.PlayerId)
	}

	return p, nil
}

func generateRoomID() uint32 {
	return rand.Uint32()
}
