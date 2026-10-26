package qr2

import (
	"encoding/binary"
	"fmt"
	"net"

	"wwfc/common"
	"wwfc/logging"
)

type ResponseFromMKWServer uint8

const (
	Ready       = 0
	PlayerAdded = 1
	AidKicked   = 2
	Log         = 0xff
)

// Indicates that mkw-server is ready to accept tcp messages from wfc-server
type ReadyResponse struct {
	responseType ResponseFromMKWServer
	port         uint16 // used to identify itself
}

// Confirms that mkw-server added a player and is ready to communicate with them
type PlayerAddedResponse struct {
	responseType ResponseFromMKWServer
	searchId     uint64
}

// Informs wfc-server that mkw-server kicked an aid on its own (e.g. too many invalid packets)
type AidKickedResponse struct {
	responseType ResponseFromMKWServer
	ip           uint32
	port         uint16
	aid          uint8
	reason       MKWServerKickReason
}

// Why mkw-server kicked an aid. Matches mkw-server's core.KickReason
type MKWServerKickReason uint8

const (
	KickReasonInvalidPackets MKWServerKickReason = 1
)

func (r MKWServerKickReason) String() string {
	switch r {
	case KickReasonInvalidPackets:
		return "invalid packets"
	default:
		return fmt.Sprintf("unknown (%d)", uint8(r))
	}
}

// In TCP, messages can get cut off in a single send and be incomplete. But they can be
// completed in subsequent receives from mkw-server. We store each mkw-servers messages
// in a buffer to be able to complete them later
var mkwServerMessageBuffer map[string]*[]byte

// key is conn.RemoteAddr().String(), which is the only unique identifier in a net.Conn
// This is fine if wfc-server and mkw-server are on the same machine and listening to messages on localhost.
// But this is very dangerous for remote mkw-servers, since the address could be spoofed.
// So this map structure must be changed when remote mkw-server is implemented

func acceptMKWServerMessages() {
	mkwServerMessageBuffer = make(map[string]*[]byte)
	for {
		conn, err := mkwServerListener.Accept()
		if err != nil {
			logging.Error(moduleName, "Error accepting connection:", err)
			continue
		}
		go readMKWServerMessage(conn)
	}
}

func readMKWServerMessage(conn net.Conn) {
	defer conn.Close()

	buffer := make([]byte, 256)
	for {
		n, err := conn.Read(buffer)
		if err != nil {
			logging.Error(moduleName, "Error reading from connection:", err)
			return
		}
		msg := buffer[:n]
		handleMessageFromMKWServer(conn, msg)
	}
}

func handleMessageFromMKWServer(conn net.Conn, msg []byte) {
	if len(msg) == 0 {
		return
	}
	remoteAddr := conn.RemoteAddr()
	if remoteAddr == nil {
		logging.Info(moduleName, "conn.RemoteAddr() == nil")
		return
	}

	addr := remoteAddr.String()
	// add whatever we got to the message buffer
	buffer := addMsgToBuffer(addr, msg)
	if buffer == nil {
		logging.Info(moduleName, addr, "returned nil buffer")
		return
	}

	// handle as many complete messages we can
	for {
		// note that this updates buffer if we can handle a message
		completeMessage := popMessage(buffer)
		if completeMessage == nil {
			break
		}
		if len(completeMessage) == 0 {
			continue
		}
		handleCompleteMessage(completeMessage, conn)
	}
}

func handleCompleteMessage(completeMessage []byte, conn net.Conn) {
	var matchRequest ResponseFromMKWServer = ResponseFromMKWServer(completeMessage[0])
	switch matchRequest {
	case Ready:
		response, err := unpackReadyResponse(completeMessage)
		if err != nil {
			logging.Info(moduleName, err.Error())
			return
		}
		logging.Info(moduleName, "Handling MKWServerResponseType.Ready")
		mkwServer := mkwServers[int(response.port)]
		if mkwServer == nil {
			logging.Info(moduleName, "No MKWServer on port", response.port)
			return
		}
		room := mkwServer.roomPointer
		if room == nil {
			logging.Info(moduleName, "mkwServer.roomPointer is nil")
			return
		}
		if room.mkwServer == nil {
			logging.Info(moduleName, "Room", room.roomID, "mkwServer is nil")
			return
		}
		room.mkwServer.conn = conn
		err = room.sendMKWServerJoinRoomForEachPlayer()
		if err != nil {
			logging.Info(moduleName, err)
			return
		}
		logging.Info(moduleName, "Handled MKWServerResponseType.Ready!")

	case PlayerAdded:
		logging.Info(moduleName, "Handling MKWServerResponseType.PlayerAdded")
		response, err := unpackPlayerAddedResponse(completeMessage)
		if err != nil {
			logging.Info(moduleName, err.Error())
			return
		}
		if len(completeMessage) != 9 {
			logging.Info(moduleName, "Invalid MKWServerResponseType.PlayerAdded len:", len(completeMessage))
			return
		}
		searchId := response.searchId
		player := playerBySearchID[searchId]
		if player == nil {
			logging.Info(moduleName, "Couldn't find player by search id! SearchId:", searchId, "while handling MKWServerResponseType.PlayerAdded")
			return
		}
		logging.Info(moduleName, "MKWServerResponseType.PlayerAdded: Found player", player.PlayerId, "by SearchId", searchId)
		room := player.roomPointer
		if room == nil {
			logging.Info(moduleName, "Player", player.PlayerId, "isn't in a room! In MKWServerResponseType.PlayerAdded")
			return
		}
		mkwServer := room.mkwServer
		if mkwServer == nil {
			logging.Info(moduleName, "Room", room.roomID, "has nil mkwServer!")
			return
		}
		common.SendPacket(ServerName, player.connIdx, packMKWServerAddressPacket(mkwServer.udpAddr))

	case AidKicked:
		response, err := unpackAidKickedResponse(completeMessage)
		if err != nil {
			logging.Info(moduleName, err.Error())
			return
		}
		handleAidKicked(response, conn)

	case Log:
		logging.Notice("MKW-Server Log", string(completeMessage[1:]))

	default:
		logging.Info(moduleName, "invalid matchRequest:", matchRequest)
	}
}

func unpackReadyResponse(completeMessage []byte) (ReadyResponse, error) {
	if len(completeMessage) != 3 {
		return ReadyResponse{}, fmt.Errorf("invalid ReadyResponse length: %d", len(completeMessage))
	}
	return ReadyResponse{
		responseType: ResponseFromMKWServer(completeMessage[0]),
		port:         binary.BigEndian.Uint16(completeMessage[1:]),
	}, nil
}

func unpackPlayerAddedResponse(completeMessage []byte) (PlayerAddedResponse, error) {
	if len(completeMessage) != 9 {
		return PlayerAddedResponse{}, fmt.Errorf("invalid PlayerAddedResponse length: %d", len(completeMessage))
	}
	return PlayerAddedResponse{
		responseType: ResponseFromMKWServer(completeMessage[0]),
		searchId:     binary.BigEndian.Uint64(completeMessage[1:]),
	}, nil
}

func unpackAidKickedResponse(completeMessage []byte) (AidKickedResponse, error) {
	if len(completeMessage) != 9 {
		return AidKickedResponse{}, fmt.Errorf("invalid AidKickedResponse length: %d", len(completeMessage))
	}
	return AidKickedResponse{
		responseType: ResponseFromMKWServer(completeMessage[0]),
		ip:           binary.BigEndian.Uint32(completeMessage[1:5]),
		port:         binary.BigEndian.Uint16(completeMessage[5:7]),
		aid:          completeMessage[7],
		reason:       MKWServerKickReason(completeMessage[8]),
	}, nil
}

// Disconnects a player that mkw-server kicked. Closing their GPCM connection goes through
// the usual logout path, which removes them from the room and sends mkw-server RemovePlayer.
func handleAidKicked(response AidKickedResponse, conn net.Conn) {
	mutex.Lock()

	room := roomByMKWServerConn(conn)
	if room == nil {
		mutex.Unlock()
		logging.Info(moduleName, "AidKicked: no room uses this mkw-server connection")
		return
	}

	player := room.findPlayerByAddrAndAid(response.ip, response.port, response.aid)
	if player == nil {
		mutex.Unlock()
		logging.Info(moduleName, "AidKicked: room", room.roomName, "has no player with aid", response.aid, "at", formatIPPort(response.ip, response.port))
		return
	}

	logging.Notice(moduleName, "mkw-server kicked player", player.PlayerId, "aid", response.aid, "from room", room.roomName, "reason:", response.reason.String())

	if player.login == nil {
		// No GPCM session to close, so remove them from qr2 directly
		removePlayer(common.MakeLookupAddr(player.Addr.String()))
		mutex.Unlock()
		return
	}

	profileID := player.login.ProfileID
	// gpErrorCallback takes the gpcm mutex, which must not be taken while holding ours
	mutex.Unlock()
	gpErrorCallback(profileID, "mkw_server_kick")
}

// Expects the mutex to be locked
func roomByMKWServerConn(conn net.Conn) *Room {
	for _, mkwServer := range mkwServers {
		if mkwServer != nil && mkwServer.conn == conn && mkwServer.roomPointer != nil {
			return mkwServer.roomPointer
		}
	}
	return nil
}

// Expects the mutex to be locked
func (r *Room) findPlayerByAddrAndAid(ip uint32, port uint16, aid uint8) *Player {
	for p := range r.players {
		pIP, pPort := common.IPFormatToInt(p.Addr.String())
		if uint32(pIP) == ip && pPort == port && p.aid == aid {
			return p
		}
	}
	return nil
}

func formatIPPort(ip uint32, port uint16) string {
	return fmt.Sprintf("%d.%d.%d.%d:%d", byte(ip>>24), byte(ip>>16), byte(ip>>8), byte(ip), port)
}
