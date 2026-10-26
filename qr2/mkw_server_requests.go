package qr2

import (
	"bytes"
	"encoding/binary"
	"errors"

	"wwfc/common"
	"wwfc/logging"
)

const MKWServerAddressPacketMagic uint32 = 0x4D4B5753 // 'MKWS'

type RequestToMKWServer uint8

const (
	AddPlayer    = 1
	RemovePlayer = 2
	KickAid      = 3
)

// this will tell mkw-server to update its state since a player joined
func (mkwServer *MKWServer) sendAddPlayerRequest(player *Player, isHost bool) error {
	logging.Info(moduleName, "Attempting to inform mkw-server of new player", player.PlayerId, "added to room", player.roomPointer.roomName, "where isHost is", isHost)

	if player == nil {
		logging.Info(moduleName, "sendAddPlayerRequest player is nil")
		return errors.New("player passed into sendAddPlayerRequest is nil!")
	}

	if mkwServer.conn == nil {
		return errors.New("The connection to mkw-server is nil! This is bad and shouldn't happen just before sending.")
	}

	addPlayerReq, err := packAddPlayerRequest(player, isHost)
	if err != nil {
		return err
	}

	// send it to mkw-server
	mkwServer.conn.Write(addPlayerReq)
	return nil
}

func (mkwServer *MKWServer) sendRemovePlayerRequest(player *Player) {
	if mkwServer.conn == nil {
		logging.Error(moduleName, "MkwServerInfo.WfcMkwServerConn is nil. Cannot send remove client message")
		return
	}
	if player == nil {
		logging.Error(moduleName, "player is nil. Cannot send remove client message")
		return
	}
	packet, err := packRemovePlayerRequest(player)
	if err != nil {
		logging.Error(moduleName, err.Error())
		return
	}
	logging.Info(moduleName, "Sending MKW-Server LeaveRoom for", player.PlayerId, "aid", player.aid)
	mkwServer.conn.Write(packet)
}

// Tells mkw-server to stop relaying a player's packets right away, before they're removed
// from the room by the usual RemovePlayer request
func (mkwServer *MKWServer) sendKickPlayerRequest(player *Player) {
	if mkwServer.conn == nil {
		logging.Error(moduleName, "mkw-server connection is nil. Cannot send kick message")
		return
	}
	if player == nil {
		logging.Error(moduleName, "player is nil. Cannot send kick message")
		return
	}
	packet, err := packKickPlayerRequest(player)
	if err != nil {
		logging.Error(moduleName, err.Error())
		return
	}
	logging.Info(moduleName, "Sending MKW-Server KickAid for", player.PlayerId, "aid", player.aid)
	mkwServer.conn.Write(packet)
}

// Sends KickAid to the mkw-server of the room the player is in, if any.
// Expects the mutex to be locked.
func (p *Player) kickFromMKWServer() {
	room := p.roomPointer
	if room == nil || room.mkwServer == nil || room.mkwServer.conn == nil {
		return
	}
	room.mkwServer.sendKickPlayerRequest(p)
}

// Called by moderation kicks so the room's mkw-server stops relaying the player
// immediately, rather than once their connection closes. Must not be called while
// holding the gpcm mutex.
func KickFromMKWServer(profileID uint32) {
	mutex.Lock()
	defer mutex.Unlock()

	login := logins[profileID]
	if login == nil || login.player == nil {
		return
	}
	login.player.kickFromMKWServer()
}

/*
// sent to mkw-server to tell it to add a new player

	type AddPlayerRequest struct {
		requestType	RequestToMKWServer // Always should be JoinRoom (1)
		ip          int32            // players ip
		port        uint16           // players port
		aid         uint8            // player's aid
		isHost      bool             // whether the player is host
		searchId    uint64           // might not be needed?
	}
*/
func packAddPlayerRequest(player *Player, isHost bool) ([]byte, error) {
	if player == nil {
		return nil, errors.New("player passed into packAddPlayerRequest is nil")
	}
	ip, port := common.IPFormatToInt(player.Addr.String())
	buf := new(bytes.Buffer)
	binary.Write(buf, binary.BigEndian, uint8(AddPlayer))
	binary.Write(buf, binary.BigEndian, ip)
	binary.Write(buf, binary.BigEndian, port)
	binary.Write(buf, binary.BigEndian, player.aid)
	binary.Write(buf, binary.BigEndian, isHost)
	binary.Write(buf, binary.BigEndian, player.SearchId)
	binary.Write(buf, binary.BigEndian, player.hasGuest())
	return buf.Bytes(), nil
}

/*
// sent to mkw-server to tell it to remove a player

	type RemovePlayerRequest struct {
		requestType RequestToMKWServer
		ip 			uint32
		port 		uint16
	}
*/
func packRemovePlayerRequest(player *Player) ([]byte, error) {
	if player == nil {
		return nil, errors.New("player passed into packRemovePlayerRequest is nil")
	}
	ip, port := common.IPFormatToInt(player.Addr.String())
	buf := new(bytes.Buffer)
	binary.Write(buf, binary.BigEndian, uint8(RemovePlayer))
	binary.Write(buf, binary.BigEndian, int32(ip))
	binary.Write(buf, binary.BigEndian, port)
	return buf.Bytes(), nil
}

/*
// sent to mkw-server to tell it to stop relaying a player now

	type KickPlayerRequest struct {
		requestType RequestToMKWServer // Always KickAid (3)
		ip 			uint32
		port 		uint16
	}
*/
func packKickPlayerRequest(player *Player) ([]byte, error) {
	if player == nil {
		return nil, errors.New("player passed into packKickPlayerRequest is nil")
	}
	ip, port := common.IPFormatToInt(player.Addr.String())
	buf := new(bytes.Buffer)
	binary.Write(buf, binary.BigEndian, uint8(KickAid))
	binary.Write(buf, binary.BigEndian, ip)
	binary.Write(buf, binary.BigEndian, port)
	return buf.Bytes(), nil
}
