package qr2

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"os"
	"time"
	"wwfc/common"
	"wwfc/logging"

	"github.com/logrusorgru/aurora/v3"
	"gvisor.dev/gvisor/pkg/sleep"
)

func printHex(data []byte) string {
	logMsg := ""
	for i := 0; i < len(data); i++ {
		if (i % 32) == 0 {
			logMsg += "\n"
		}
		logMsg += fmt.Sprintf("%02x ", data[i])
	}

	return logMsg
}

func SendClientMessage(senderIP string, destSearchID uint64, message []byte) {
	moduleName := "QR2/MSG"

	var matchData common.MatchCommandData
	var receiver *Player

	useSearchID := destSearchID < (1 << 24)
	mutex.Lock()
	if useSearchID {
		receiver = playerBySearchID[destSearchID]
	} else {
		// It's an IP address, used in some circumstances
		receiver = players[destSearchID]
	}

	if receiver == nil || !receiver.Authenticated {
		mutex.Unlock()
		logging.Error(moduleName, "Destination", aurora.Cyan(destSearchID), "does not exist")
		return
	}

	if destPid, ok := receiver.Data["dwc_pid"]; !ok || destPid == "" {
		mutex.Unlock()
		logging.Error(moduleName, "Destination", aurora.Cyan(destSearchID), "has no profile ID")
		return
	}
	mutex.Unlock()

	login := receiver.login
	if login == nil || !login.DeviceAuthenticated {
		logging.Error(moduleName, "Destination", aurora.Cyan(destSearchID), "is not device authenticated")
		return
	}

	// Decode and validate the message
	mutex.Lock()

	destPid, ok := receiver.Data["dwc_pid"]
	if !ok || destPid == "" {
		destPid = "<UNKNOWN>"
	}

	destPlayerID := receiver.PlayerId
	packetCount := receiver.PacketCount + 1
	receiver.PacketCount = packetCount
	destAddr := receiver.Addr

	defer mutex.Unlock()

	cmd := message[8]
	common.LogMatchCommand(moduleName, destPid, cmd, matchData)

	payload := createResponseHeader(ClientMessageRequest, destPlayerID)

	payload = append(payload, []byte{0, 0, 0, 0}...)
	binary.BigEndian.PutUint32(payload[len(payload)-4:], packetCount)
	payload = append(payload, message...)

	receiver.messageMutex.Lock()
	defer receiver.messageMutex.Unlock()

	if receiver.login == nil {
		return
	}

	s := sleep.Sleeper{}
	defer s.Done()

	receiver.messageAckWaker.Clear()
	s.AddWaker(receiver.messageAckWaker)

	timeWaker := sleep.Waker{}
	s.AddWaker(&timeWaker)

	timeOutCount := 0
	for {
		time.AfterFunc(1*time.Second, func() {
			timeWaker.Assert()
		})

		_, err := masterConn.WriteTo(payload, &destAddr)
		if err != nil {
			logging.Error(moduleName, "Error sending message:", err.Error())
		}

		// Wait for an ack or timeout
		switch s.Fetch(true) {
		case &timeWaker:
			timeOutCount++

			// Enforce a 10 second timeout
			if timeOutCount <= 10 {
				break
			}

			logging.Error(moduleName, "Timed out waiting for ack")
			// Kick the player
			if login := receiver.login; login != nil {
				receiver.kickFromMKWServer()
				gpErrorCallback(login.ProfileID, "network_error")
				receiver.login = nil
			}
			return

		default:
			return
		}
	}
}

func sendClientExploit(moduleName string, playerCopy Player) {
	if len(playerCopy.login.GameCode) != 4 || !common.IsUppercaseAlphanumeric(playerCopy.login.GameCode) {
		logging.Error(moduleName, "Invalid game code:", aurora.Cyan(playerCopy.login.GameCode))
		return
	}

	exploit, err := os.ReadFile("payload/sbcm/" + "payload." + playerCopy.login.GameCode + ".bin")
	if err != nil {
		logging.Error(moduleName, "Error reading exploit file", aurora.Cyan(playerCopy.login.GameCode), "-", err.Error())
		return
	}

	mutex.Lock()
	player, playerExists := players[common.MakeLookupAddr(playerCopy.Addr.String())]
	if !playerExists {
		mutex.Unlock()
		logging.Error(moduleName, "Player not found")
		return
	}

	packetCount := player.PacketCount + 1
	player.PacketCount = packetCount
	mutex.Unlock()

	// Now send the exploit
	payload := createResponseHeader(ClientMessageRequest, playerCopy.PlayerId)
	payload = append(payload, []byte{0, 0, 0, 0}...)
	binary.BigEndian.PutUint32(payload[len(payload)-4:], packetCount)
	payload = append(payload, exploit[0xB:]...)

	go func() {
		for {
			_, err = masterConn.WriteTo(payload, &playerCopy.Addr)
			if err != nil {
				logging.Error(moduleName, "Error sending message:", err.Error())
			}

			// Resend the message if no ack after 2 seconds
			time.Sleep(2 * time.Second)

			mutex.Lock()
			player, playerExists := players[common.MakeLookupAddr(playerCopy.Addr.String())]
			if !playerExists || player.ExploitReceived || player.login == nil || !player.login.NeedsExploit {
				mutex.Unlock()
				return
			}

			mutex.Unlock()
			logging.Notice(moduleName, "Resending SBCM exploit to DNS patcher client")
		}
	}()
}

func sendPlayerSearchId(player *Player) {
	packet := SearchIdPacket{
		Magic:    [8]uint8{'S', 'E', 'A', 'R', 'C', 'H', 'I', 'D'},
		SearchId: player.SearchId,
	}

	buf := new(bytes.Buffer)
	binary.Write(buf, binary.BigEndian, packet)

	go func() {
		for {
			_, err := masterConn.WriteTo(buf.Bytes(), &player.Addr)
			if err != nil {
				logging.Info(moduleName, "Error sending search ID:", err.Error())
			}

			time.Sleep(3 * time.Second)

			player = players[common.MakeLookupAddr(player.Addr.String())]
			if player == nil || player.recvSearchId {
				return
			}

			if player.searchIdGuesses >= 5 {
				logging.Info(moduleName, "Reached max search ID resends for player", aurora.Cyan(player.PlayerId))
				return
			}

			if player.recvSearchId {
				logging.Info(moduleName, "Player acknowledged search ID, stopping resends")
				return
			}
		}

	}()

}
