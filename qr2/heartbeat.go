package qr2

import (
	"encoding/binary"
	"net"
	"strconv"
	"strings"
	"wwfc/common"
	"wwfc/logging"

	"github.com/logrusorgru/aurora/v3"
)

func heartbeat(moduleName string, conn net.PacketConn, addr net.UDPAddr, buffer []byte) {
	playerId := binary.BigEndian.Uint32(buffer[1:5])
	values := strings.Split(string(buffer[5:]), "\u0000")

	payload := map[string]string{}
	unknowns := []string{}
	for i := 0; i < len(values); i += 2 {
		if len(values[i]) == 0 || values[i][0] == '+' {
			continue
		}

		if values[i] == "unknown" {
			unknowns = append(unknowns, values[i+1])
			continue
		}

		payload[values[i]] = values[i+1]
	}

	if payload["dwc_mtype"] != "" {
		logging.Info(moduleName, "Match type:", aurora.Cyan(payload["dwc_mtype"]))
	}

	if payload["dwc_hoststate"] != "" {
		logging.Info(moduleName, "Host state:", aurora.Cyan(payload["dwc_hoststate"]))
	}

	realIP, realPort := common.IPFormatToString(addr.String())

	noIP := false
	if ip, ok := payload["publicip"]; !ok || ip == "0" {
		noIP = true
	}

	clientEndianness := common.GetExpectedUnitCode(payload["gamename"])
	if !noIP && clientEndianness == ClientBigEndian {
		if payload["publicip"] != realIP || payload["publicport"] != realPort {
			// Client is mistaken about its public IP
			logging.Error(moduleName, "Public IP mismatch")
			return
		}
	} else if !noIP && clientEndianness == ClientLittleEndian {
		realIPLE, realPortLE := common.IPFormatToStringLE(addr.String())
		if payload["publicip"] != realIPLE || payload["publicport"] != realPortLE {
			// Client is mistaken about its public IP
			logging.Error(moduleName, "Public IP mismatch")
			return
		}
	}
	// Else it's a cross-compatible game and the endianness is ambiguous

	payload["publicip"] = realIP
	payload["publicport"] = realPort

	lookupAddr := common.MakeLookupAddr(addr.String())

	statechanged, ok := payload["statechanged"]
	if ok && statechanged == "2" {
		logging.Notice(moduleName, "Client player shutdown")
		mutex.Lock()
		removePlayer(lookupAddr)
		mutex.Unlock()
		return
	}

	if ratingError := checkValidRating(moduleName, payload); ratingError != "ok" {
		mutex.Lock()
		player, playerExists := players[lookupAddr]
		if playerExists && player.login != nil {
			profileId := player.login.ProfileID

			mutex.Unlock()
			gpErrorCallback(profileId, ratingError)
			return
		} else {
			// Else don't return and move on, so we can return an error once logged in
			mutex.Unlock()
		}
	}

	player, ok := setPlayerData(moduleName, &addr, playerId, payload)
	if !ok {
		return
	}

	if len(unknowns) > 0 {
		// Try to login using the first unknown as a profile ID
		// This makes it possible to execute the exploit on the client sooner

		mutex.Lock()
		playerPtr, playerExists := players[lookupAddr]
		if !playerExists {
			logging.Error(moduleName, "Player not found")
		} else if playerPtr.login == nil {
			profileId := unknowns[0]
			playerPtr.setProfileID(moduleName, profileId, "")
		}
		player = *playerPtr
		mutex.Unlock()
	}

	if !player.Authenticated || noIP {
		sendChallenge(conn, addr, player, lookupAddr)
	}

	if login := player.login; !player.ExploitReceived && login != nil && player.login.NeedsExploit {
		// The version of DWC in Mario Kart DS doesn't check matching status
		if (!noIP && statechanged == "1") || login.GameCode == "AMCE" || login.GameCode == "AMCP" || login.GameCode == "AMCJ" {
			logging.Notice(moduleName, "Sending SBCM exploit to DNS patcher client")
			sendClientExploit(moduleName, player)
		}
	}

	// try to send the player their search id
	if player.SearchId != 0 && !player.recvSearchId && player.searchIdGuesses < 5 {
		sendPlayerSearchId(&player)
	}
}

func checkValidRating(moduleName string, payload map[string]string) string {
	if payload["gamename"] == "mariokartwii" {
		// ev and eb values must be in range 1 to 65535 (VR is a u16; mogi rooms send LR, which can
		// pass 9999, in its place)
		if ev := payload["ev"]; ev != "" {
			evInt, err := strconv.ParseInt(ev, 10, 32)
			if err != nil || evInt < 1 || evInt > 65535 {
				logging.Error(moduleName, "Invalid ev value:", aurora.Cyan(ev))
				return "invalid_elo"
			}
		}

		if eb := payload["eb"]; eb != "" {
			ebInt, err := strconv.ParseInt(eb, 10, 32)
			if err != nil || ebInt < 1 || ebInt > 65535 {
				logging.Error(moduleName, "Invalid eb value:", aurora.Cyan(eb))
				return "invalid_elo"
			}
		}
	}

	return "ok"
}
