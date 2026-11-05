package qr2

import (
	"errors"
	"fmt"

	"wwfc/common"
	"wwfc/logging"

	"github.com/logrusorgru/aurora/v3"
)

// request to open a private room

func handleOpenRoomRequest(host *Player, mogi bool) error {
	if mogi {
		// Not enforced yet: the public Lounge API only has host friend codes, so most
		// players aren't found. (enforceMogiLinks)
		name, ok := mogiLinked(host)
		if !ok {
			kickUnlinked(host)
			return fmt.Errorf("player %d can't host a mogi: no linked Lounge profile", host.PlayerId)
		}
		logging.Info(moduleName, "Player", host.PlayerId, "hosts a mogi; Lounge player:", name, ok)
	}
	err := createRoom(host, common.Private, common.Undecided)
	if err != nil {
		return fmt.Errorf("createRoom() failed for player %d with message %s", host.PlayerId, err.Error())
	}
	if r := host.roomPointer; r != nil {
		r.mogi = mogi
		r.broadcastLoungeRanks()
	}
	return nil
}

func handleJoinFriendRequest(joiner *Player, request *JoinFriendRequest) error {
	logging.Info(moduleName, "Player wants to join a friend", aurora.Cyan(request.friendProfileId), "and searchRegion", request.searchRegion)

	l := logins[request.friendProfileId]
	if l == nil {
		return errors.New("Login with friendProfileId doesn't exist")
	}

	err := joiner.canJoinFriend(l)
	if err != nil {
		return err
	}

	friend := l.player
	r := friend.roomPointer
	if r == nil {
		return errors.New("Friend isn't in a room")
	}

	if r.mogi {
		name, ok := mogiLinked(joiner)
		if !ok {
			kickUnlinked(joiner)
			return fmt.Errorf("player %d can't join mogi %s: no linked Lounge profile", joiner.PlayerId, r.roomName)
		}
		logging.Info(moduleName, "Player", joiner.PlayerId, "joins mogi", r.roomName, "; Lounge player:", name, ok)
	}

	if r.Region != request.searchRegion {
		return fmt.Errorf("Region mismatch. Requested is %d while friend's is %d", request.searchRegion, r.Region)
	}

	if r.Region == common.Private && r.host != friend {
		return errors.New("Rooms host isn't the expected host!")
	}

	// TODO: Rest of this function is awkward to read
	err = r.tryAddPlayer(joiner, false)
	if err == nil {
		return nil
	} else {
		logging.Info(moduleName, err.Error())
	}

	// Note: Vanilla MKW doesn't allow joining a friend's public room if suspended. Handle on backend anyway.
	err = r.tryAddWaitingPlayer(joiner)
	if err == nil {
		return nil
	} else {
		logging.Info(moduleName, err.Error())
	}

	return fmt.Errorf("Player %d is unable to join friend player %d's room %s", joiner.PlayerId, friend.PlayerId, r.roomName)
}

func handleLeaveRoomRequest(p *Player) error {
	r := p.roomPointer
	if r != nil {
		return r.removePlayer(p)
	}

	r = p.waitingRoom
	if r != nil {
		return r.removeWaitingPlayer(p)
	}

	return fmt.Errorf("Player %d sent a LeaveRoom request when they're roomless!", p.PlayerId)
}

func handleSuspendRequest(p *Player, requestSuspend bool) error {
	if p.suspendVote == requestSuspend {
		return nil
	}

	r := p.roomPointer
	if r == nil {
		// Ideally an error would be returned, but the client can't distinguish between
		// a graceful race end and a ftw dc. In the latter case, they will spam suspend requests.
		// So return nil until a client patch is made to scilence noise. This is a todo
		return nil
	}

	p.suspendVote = requestSuspend

	// Try to update room suspension
	r.updateSuspension()
	return nil
}

func handleSearchPublicRoomRequest(p *Player, searchReq *SearchPublicRoomRequest) error {
	if p.roomPointer != nil {
		return fmt.Errorf("Player %d requested to search for a room, but their roomPointer isn't nil (room %d)", p.PlayerId, p.roomPointer.roomID)
	}

	return findPublicRoom(p, searchReq.region, searchReq.mode)
}

func handleSetLocalPlayerCount(p *Player, lpc uint8) error {
	err := p.setLocalPlayers(lpc)

	return err
}
