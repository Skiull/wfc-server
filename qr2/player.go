package qr2

import (
	"encoding/gob"
	"errors"
	"fmt"
	"math/rand"
	"net"
	"os"
	"strconv"
	"strings"
	"time"
	"wwfc/common"
	"wwfc/logging"

	"github.com/logrusorgru/aurora/v3"
	"github.com/sasha-s/go-deadlock"
	"gvisor.dev/gvisor/pkg/sleep"
)

const (
	ClientLittleEndian = 0
	ClientBigEndian    = 1
	ClientNoEndian     = 2
)

const MaxAid = 11
const NoAid = 0xff

type Player struct {
	PlayerId        uint32
	SearchId        uint64
	Addr            net.UDPAddr
	Challenge       string
	Authenticated   bool
	login           *LoginInfo
	ExploitReceived bool
	LastKeepAlive   int64
	Data            map[string]string
	PacketCount     uint32
	messageMutex    *deadlock.Mutex
	messageAckWaker *sleep.Waker
	roomPointer     *Room
	waitingRoom     *Room
	RoomName        string

	recvSearchId    bool
	searchIdGuesses uint32 // attempt to prevent brute forcing the searchId

	aid              uint8  // only set when in a room
	suspendVote      bool   // vote to suspend match making.
	replayChoice     int    // Replay picked in the Replays menu (index into ownReplays).
	showLoungeRank   bool   // Ranks setting: show this player's Lounge rank (lounge_rank.go).
	localPlayerCount uint32 // 4 bytes rather than 1 since it has to be represented in little endian

	// Test bot played by mkw-server. Bots only exist inside a room.
	isBot bool

	// Room Manager fields
	connIdx         uint64
	roomManagerAddr string
}

var (
	players          = map[uint64]*Player{}
	playerBySearchID = map[uint64]*Player{}
	mutex            = deadlock.Mutex{}
)

func (p *Player) setRoomInfo(r *Room, aid uint8) {
	if r == nil {
		logging.Info(moduleName, "Can't set player's room info, room is nil!")
		return
	}

	if !r.players[p] {
		logging.Info(moduleName, "Player not in room, can't set room info!")
		return
	}

	p.roomPointer = r
	p.RoomName = r.roomName
	p.aid = aid
}

// sets Player fields related to being in a room. aid, roomPointer, etc.
func (p *Player) resetRoomInfo() {
	p.aid = NoAid
	p.suspendVote = false

	p.roomPointer = nil
}

// Remove a player from players. Called upon leaving wfc, disconnect, power off.
// Expects the global mutex to already be locked.
func removePlayer(addr uint64) {
	player := players[addr]
	if player == nil {
		return
	}

	player.messageAckWaker.Assert()

	// remove player from room if they're in one
	room := player.roomPointer
	if room != nil {
		err := room.removePlayer(player)
		if err != nil {
			logging.Error(moduleName, "player.removePlayer() failed to remove PlayerId", player.PlayerId, "with reason", err.Error())
		}
	}

	// remove player from room they're waiting to join
	if player.waitingRoom != nil {
		err := player.waitingRoom.removeWaitingPlayer(player)
		if err != nil {
			logging.Info(moduleName, "player.removePlayer() failed to remove from waiting list. Reason:", err.Error())
		}
	}

	if player.login != nil {
		player.login.player = nil
		player.login = nil
	}

	// Delete search ID lookup
	delete(playerBySearchID, players[addr].SearchId)

	delete(players, addr)
}

func (p *Player) setWaitingRoom(r *Room) {
	p.waitingRoom = r
}

func (p *Player) localPlayerCountOk() bool {
	return p.localPlayerCount == 1 || p.localPlayerCount == 2
}

func (p *Player) sendReliableMsgToPlayer(msg []byte) error {
	return common.SendPacket(ServerName, p.connIdx, msg)
}

func (p *Player) setRoomManagerConnection(connectionIndex uint64, address string) {
	p.connIdx = connectionIndex
	p.roomManagerAddr = address
}

// Update player data, creating the player if it doesn't exist. Returns a copy of the player data.
func setPlayerData(moduleName string, addr net.Addr, playerId uint32, payload map[string]string) (Player, bool) {
	newPID, newPIDValid := payload["dwc_pid"]
	delete(payload, "dwc_pid")

	lookupAddr := common.MakeLookupAddr(addr.String())

	// Moving into performing operations on the player data, so lock the mutex
	mutex.Lock()
	defer mutex.Unlock()
	player, playerExists := players[lookupAddr]

	if playerExists && player.Addr.String() != addr.String() {
		logging.Error(moduleName, "player IP mismatch")
		return Player{}, false
	}

	if !playerExists {
		player = &Player{
			PlayerId:        playerId,
			Addr:            *addr.(*net.UDPAddr),
			Challenge:       "",
			Authenticated:   false,
			LastKeepAlive:   time.Now().UTC().Unix(),
			Data:            payload,
			PacketCount:     0,
			messageMutex:    &deadlock.Mutex{},
			messageAckWaker: &sleep.Waker{},
			recvSearchId:    false,
			searchIdGuesses: 0,
		}
	}

	if newPIDValid && !player.setProfileID(moduleName, newPID, "") {
		return Player{}, false
	}

	if !playerExists {
		logging.Info(moduleName, "Creating PlayerId", aurora.Cyan(playerId).String(), "for", addr.String())

		// Set search ID
		for {
			searchID := uint64(rand.Int63n((1<<24)-1) + 1)
			if _, exists := playerBySearchID[searchID]; !exists {
				player.SearchId = searchID
				player.Data["+searchid"] = strconv.FormatUint(searchID, 10)
				playerBySearchID[searchID] = player
				logging.Info(moduleName, "Assigning PlayerId", player.PlayerId, "SearchId:", searchID)
				break
			}
		}

		players[lookupAddr] = player
		return *player, true
	}

	// Save certain fields
	for k, v := range player.Data {
		if k[0] == '+' || k == "dwc_pid" {
			payload[k] = v
		}
	}

	player.Data = payload
	player.LastKeepAlive = time.Now().UTC().Unix()
	player.PlayerId = playerId
	return *player, true
}

// Set the player's profile ID if it doesn't already exists.
// Returns false if the profile ID is invalid.
// Expects the global mutex to already be locked.
func (player *Player) setProfileID(moduleName string, newPID string, gpcmIP string) bool {
	if oldPID, oldPIDValid := player.Data["dwc_pid"]; oldPIDValid && oldPID != "" {
		if newPID != oldPID {
			logging.Error(moduleName, "New dwc_pid mismatch: new:", aurora.Cyan(newPID), "old:", aurora.Cyan(oldPID))
			return false
		}

		return true
	}

	// Setting a new PID so validate it
	profileID, err := strconv.ParseUint(newPID, 10, 32)
	if err != nil || strconv.FormatUint(profileID, 10) != newPID {
		logging.Error(moduleName, "Invalid dwc_pid value:", aurora.Cyan(newPID))
		return false
	}

	// Check if the public IP matches the one used for the GPCM player
	var gpPublicIP string
	var loginInfo *LoginInfo
	var ok bool
	if loginInfo, ok = logins[uint32(profileID)]; ok {
		gpPublicIP = strings.Split(loginInfo.GPPublicIP, ":")[0]
	} else {
		logging.Error(moduleName, "Provided dwc_pid is not logged in:", aurora.Cyan(newPID))
		return false
	}

	// TODO: Some kind of authentication
	if gpcmIP != "" && gpcmIP != gpPublicIP {
		logging.Error(moduleName, "TCP public IP mismatch: SB:", aurora.Cyan(gpcmIP), "GP:", aurora.Cyan(gpPublicIP))
		return false
	}

	if ratingError := checkValidRating(moduleName, player.Data); ratingError != "ok" {
		profileId := loginInfo.ProfileID

		mutex.Unlock()
		gpErrorCallback(profileId, ratingError)
		mutex.Lock()
		return false
	}

	player.login = loginInfo

	// Constraint: only one player can exist with a given profile ID
	if loginInfo.player != nil {
		logging.Notice(moduleName, "Removing outdated player", aurora.BrightCyan(loginInfo.player.Addr.String()), "with PID", aurora.Cyan(newPID))
		removePlayer(common.MakeLookupAddr(loginInfo.player.Addr.String()))
	}

	loginInfo.player = player

	if loginInfo.DeviceAuthenticated {
		player.Data["+deviceauth"] = "1"
	} else {
		player.Data["+deviceauth"] = "0"
	}

	player.Data["+gppublicip"], _ = common.IPFormatToString(gpPublicIP)
	player.Data["+fcgameid"] = loginInfo.FriendKeyGame

	player.Data["dwc_pid"] = newPID
	logging.Notice(moduleName, "Opened player", player.PlayerId, "with PID", aurora.Cyan(newPID))

	return true
}

// Get a copy of the list of servers
func GetPlayerServers() []map[string]string {
	var servers []map[string]string
	var unreachable []uint64
	currentTime := time.Now().UTC().Unix()

	mutex.Lock()
	defer mutex.Unlock()
	for playerAddr, player := range players {
		// If the last keep alive was over a minute ago then consider the server unreachable
		if player.LastKeepAlive < currentTime-60 {
			// If the last keep alive was over an hour ago then remove the server
			if player.LastKeepAlive < currentTime-((60*60)*1) {
				unreachable = append(unreachable, playerAddr)
			}
			continue
		}

		if !player.Authenticated {
			continue
		}

		servers = append(servers, player.Data)
	}

	// Remove unreachable players
	for _, playerAddr := range unreachable {
		logging.Notice("QR2", "Removing unreachable player", aurora.BrightCyan(players[playerAddr].Addr.String()))
		removePlayer(playerAddr)
	}

	return servers
}

func (p *Player) canCreateRoom() error {
	if p.roomPointer != nil {
		return fmt.Errorf("Already in a room (%d)", p.roomPointer.roomID)
	}

	if !p.localPlayerCountOk() {
		return fmt.Errorf("p.localPlayerCountOk() failed where p.localPlayerCount: %d", p.localPlayerCount)
	}

	return nil
}

func (p *Player) setLocalPlayers(lpc uint8) error {
	// Only values of 1 or two players is valid
	if lpc != 1 && lpc != 2 {
		return fmt.Errorf("Player %d sent in invalid player count (%d)", p.PlayerId, lpc)
	}

	// Local player count can only be set once per session. 0 is a default value to check that against.
	if p.localPlayerCount != 0 {
		return fmt.Errorf("Player %d already had their player count set!", p.PlayerId)
	}

	p.localPlayerCount = uint32(lpc)

	return nil
}

func (p *Player) hasGuest() bool {
	return p.localPlayerCount == 2
}

// Save the players to a file. Expects the mutex to be locked.
func savePlayers() error {
	file, err := os.OpenFile("state/qr2_players.gob", os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		return err
	}

	encoder := gob.NewEncoder(file)
	err = encoder.Encode(players)
	file.Close()
	return err
}

// Load the players from a file. Expects the mutex to be locked.
func loadPlayers() error {
	file, err := os.Open("state/qr2_players.gob")
	if err != nil {
		return err
	}

	decoder := gob.NewDecoder(file)
	err = decoder.Decode(&players)
	file.Close()
	if err != nil {
		return err
	}

	for _, players := range players {
		if players.SearchId != 0 {
			playerBySearchID[players.SearchId] = players
		}

		players.messageMutex = &deadlock.Mutex{}
		players.messageAckWaker = &sleep.Waker{}
		players.roomPointer = nil
		players.login = nil
	}

	return nil
}

// Returns nil when either:
// - open-host is enabled
// - both players have each other added
func (p *Player) canJoinFriend(l *LoginInfo) error {
	// check for open host
	if l.OpenHost {
		return nil
	}

	if p.login == nil {
		return errors.New("Player's login is nil")
	}

	for _, friendsFriend := range l.friendsList {
		if friendsFriend == p.login.ProfileID {
			return nil
		}
	}

	return errors.New("Player's aren't mutural friends and host has open-host disabled")
}
