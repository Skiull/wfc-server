package qr2

import (
	"encoding/binary"
	"fmt"

	"wwfc/common"
)

type MatchRequestType uint8

// These are requests players send to wfc-server
const (
	OpenRoom         = 0
	JoinFriend       = 1
	LeaveRoom        = 2
	Suspend          = 3
	SearchPublicRoom = 4
	LocalPlayerCount = 5
	LeaderboardPage  = 6 // u16 page at 0x10 (leaderboard.go)
	ReplayList       = 7 // replay_list.go
	OpenMogi         = 8 // Like OpenRoom, for a mogi room (lounge_fc.go)
	MogiList         = 9 // mogi.go
	RankSetting      = 10 // u8 at 0x10: show my Lounge rank (lounge_rank.go)
)

type MatchRequestHeader struct {
	Magic       uint32 // "MREQ"
	requestType MatchRequestType
	padding     [3]uint8
	searchId    uint64
}

const MatchRequestHeaderSize int = 0x10

type JoinFriendRequest struct {
	header          MatchRequestHeader
	friendProfileId uint32
	searchRegion    common.MKWServerSearchRegion
}

type SuspendRequest struct {
	header         MatchRequestHeader
	suspendRequest bool
}

type SearchPublicRoomRequest struct {
	header MatchRequestHeader
	region common.MKWServerSearchRegion
	mode   common.MKWServerGameMode
	replay int // Region common.Replay: which of the player's replays (0: newest).
}

const MatchRequestHeaderMagic string = "MREQ"

func tryParseMatchRequestHeader(data []byte) (*MatchRequestHeader, error) {
	if len(data) != MatchRequestHeaderSize {
		return nil, fmt.Errorf("Received packet with invalid length %d expected 16", len(data))
	}

	magic := data[:4]
	if string(magic) != MatchRequestHeaderMagic {
		return nil, fmt.Errorf("Received packet with invalid magic (%s) expected MREQ", magic)
	}

	matchRequest := uint8(data[4])

	return &MatchRequestHeader{
		Magic:       binary.BigEndian.Uint32([]byte(MatchRequestHeaderMagic)),
		requestType: MatchRequestType(matchRequest),
		searchId:    binary.BigEndian.Uint64(data[8:16]),
	}, nil
}
