package qr2

import (
	"encoding/binary"
	"strings"
	"unicode/utf16"

	"wwfc/common"
)

// Mogis: friend rooms that only players linked to an MKW Lounge profile can create or join
// (Mogi button, then Create a Mogi: OpenMogi). The Mogi button first asks whether the player
// is linked (MogiList), to tell them instead of starting a room it would refuse.

const (
	MogiListPacketMagic = 0x4D4F474C // 'MOGL'
	maxMogiListed       = 10
	mogiNameLen         = 16
	mogiEntrySize       = 4 + 1 + 3 + mogiNameLen*2
)

// mogiLinked reports whether a player may create or join mogis, and their Lounge name.
func mogiLinked(p *Player) (string, bool) {
	if name, ok := playerLoungeName(p); ok {
		return name, true
	}
	if p.login != nil {
		fc := common.CalcFriendCode(p.login.ProfileID, "RMCJ")
		for _, s := range strings.Split(common.GetConfig().MogiTestFriendCodes, ",") {
			if c, ok := parseFriendCode(s); ok && c == fc {
				return "", true
			}
		}
	}
	return "", false
}

// Packet: magic, u8 linked, u8 count, 2 bytes padding, then 10 entries of
//
//	u32 room id, u8 players, 3 bytes padding, u16 host name[16] (Lounge name, UTF-16BE)
func packMogiList(p *Player) []byte {
	buf := make([]byte, 8+maxMogiListed*mogiEntrySize)
	binary.BigEndian.PutUint32(buf[0:], MogiListPacketMagic)
	if _, ok := mogiLinked(p); ok {
		buf[4] = 1
	}
	n := 0
	for _, r := range rooms {
		if n >= maxMogiListed || r == nil || !r.mogi || r.canceled || r.host == nil {
			continue
		}
		e := buf[8+n*mogiEntrySize:]
		binary.BigEndian.PutUint32(e[0:], r.roomID)
		e[4] = byte(r.numPlayers())
		name, _ := playerLoungeName(r.host)
		if name == "" {
			name = "Mogi " + r.roomName
		}
		u := utf16.Encode([]rune(name))
		for j := 0; j < len(u) && j < mogiNameLen; j++ {
			binary.BigEndian.PutUint16(e[8+j*2:], u[j])
		}
		n++
	}
	buf[5] = byte(n)
	return buf
}

func handleMogiListRequest(p *Player) error {
	return common.SendPacket(ServerName, p.connIdx, packMogiList(p))
}
