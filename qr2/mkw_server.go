package qr2

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"strings"
	"syscall"

	"wwfc/common"
	"wwfc/logging"
)

type MKWServer struct {
	// process of the mkw server. i think this wouldnt work for remove servers
	cmd         *exec.Cmd
	isRemote    bool        // TODO: Unused for now
	udpAddr     net.UDPAddr // the udp address of the mkw server (clients send/receive here)
	conn        net.Conn    // connection to the mkw server's WFC listener
	roomPointer *Room
}

// key is the room address, easy for clients/rooms to lookup
var mkwServers = map[int]*MKWServer{}

func startMKWServer(r *Room) (*MKWServer, error) {
	logging.Info(moduleName, "Attempting to start mkw-server process for room", r.roomName)
	// for now just use the gamespy address. Only works if wfc-server and mkw-server are on the same machine
	mkwServerIP := net.ParseIP(*common.GetConfig().GameSpyAddress)
	if mkwServerIP == nil {
		return nil, errors.New("failed to parse mkw-server's ip from the config. Please verify it's formatted correctly!")
	}

	// use a random port to be less predictable and also avoid conflicts
	mkwServerPort, err := common.FindOpenUDPPort(26000, 26999)
	if err != nil {
		return nil, fmt.Errorf("failed to find open UDP port: %w", err)
	}

	mkwServerAddr := net.UDPAddr{
		IP:   mkwServerIP,
		Port: mkwServerPort,
	}

	mkwServerPath := common.GetConfig().MkwServerPath

	args := []string{
		"--room-addr", mkwServerAddr.String(),
		"--wfc-addr", mkwServerListener.Addr().String(),
	}
	args = append(args, strings.Fields(common.GetConfig().MkwServerArgs)...)
	if r.replay != nil {
		args = append(args, "-replay", r.replay.file, "-replay-spectate")
	}

	cmd := exec.Command(mkwServerPath, args...)

	logging.Info(moduleName, "Running command to start mkw-server process:", cmd)

	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("failed to start mkw-server process: %w", err)
	}

	go func(c *exec.Cmd) {
		err := c.Wait()
		if err != nil {
			logging.Error(moduleName, "mkw-server process exited with error:", err)
		} else {
			logging.Info(moduleName, "mkw-server process exited successfully")
		}
		// Close the room if mkw-server ends (if not already closed).
		// fixme: This isn't perfect for players searching for a public room since they'll
		// silently not be able to find a room until they exit and re-enter the match making state.
		// All other players just disconnect.
		if !r.canceled {
			r.close()
		}

	}(cmd)

	mkwServer := &MKWServer{
		cmd:         cmd,
		isRemote:    false,
		udpAddr:     mkwServerAddr, // set later by ROOM_OPEN message
		conn:        nil,           // set later by ROOM_OPEN message
		roomPointer: r,
	}

	mkwServers[mkwServerPort] = mkwServer

	logging.Info(moduleName, "Created mkw-server for room", r.roomID, "at TCP port", mkwServerPort)

	return mkwServer, nil
}

func (mkwServer *MKWServer) terminateProcess() {
	if mkwServer.cmd == nil || mkwServer.cmd.Process == nil {
		logging.Info(moduleName, "Can't terminate mkw-server process, its already nil (already terminated?)")
		return
	}

	// Windows doesn't support SIGTERM, so fall back to killing the process.
	if err := mkwServer.cmd.Process.Signal(syscall.SIGTERM); err != nil {
		if err := mkwServer.cmd.Process.Kill(); err != nil {
			logging.Error(moduleName, "Failed to terminate mkw-server process:", err)
			return
		}
	}
	logging.Info(moduleName, "Terminated MKW-Server process!")
}

/*
// sent to clients to let them know the udp address of mkw-server

	type MKWServerAddressPacket struct {
		magic uint32
		ip    int32
		port  uint16
	}
*/
// addrFor returns the room address to send a player. mkw-server may listen on every interface
// (0.0.0.0), which isn't an address a player can send to.
func (mkwServer *MKWServer) addrFor(player *Player) net.UDPAddr {
	addr := mkwServer.udpAddr
	if player.Addr.IP.IsLoopback() {
		addr.IP = net.IPv4(127, 0, 0, 1)
	} else if public := net.ParseIP(common.GetConfig().MkwServerPublicAddress); public != nil {
		addr.IP = public
	} else if addr.IP.IsUnspecified() {
		addr.IP = net.IPv4(127, 0, 0, 1)
	}
	return addr
}

func packMKWServerAddressPacket(addr net.UDPAddr) []byte {
	ip, port := common.IPFormatToInt(addr.String())
	buf := new(bytes.Buffer)
	binary.Write(buf, binary.BigEndian, MKWServerAddressPacketMagic)
	binary.Write(buf, binary.BigEndian, ip)
	binary.Write(buf, binary.BigEndian, port)
	return buf.Bytes()
}
