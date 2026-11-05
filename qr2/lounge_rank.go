package qr2

import (
	"encoding/binary"
	"strings"

	"wwfc/common"
)

// Lounge ranks on the in-race name tags: each player can choose (Ranks setting) to show their
// Lounge rank instead of the star rank. wfc-server sends everyone in a room the rank of each
// aid that chose it, and the client draws the rank icon on that player's name tag.

const LoungeRanksPacketMagic = 0x524E4B53 // 'RNKS'

// loungeRankOf is the player's Lounge rank (index into leaderboardRanks), from their friend
// code, or from <loungeTestPlayers>.
func loungeRankOf(p *Player) (int, bool) {
	lr, ok := loungeLROf(p)
	return rankForLR(lr), ok
}

// loungeLROf is the player's Lounge LR, from their friend code or <loungeTestPlayers>.
func loungeLROf(p *Player) (int, bool) {
	if p == nil || p.login == nil {
		return 0, false
	}
	fc := common.CalcFriendCode(p.login.ProfileID, "RMCJ")
	testName := ""
	for _, pair := range strings.Split(common.GetConfig().LoungeTestPlayers, ",") {
		code, name, ok := strings.Cut(pair, "=")
		if c, okc := parseFriendCode(code); ok && okc && c == fc {
			testName = strings.TrimSpace(name)
		}
	}
	refreshLeaderboard()
	leaderboard.Lock()
	defer leaderboard.Unlock()
	for _, e := range leaderboard.entries {
		if (testName != "" && strings.EqualFold(e.name, testName)) || (testName == "" && e.fc == fc) {
			return e.lr, true
		}
	}
	return 0, false
}

// Packet: magic, then 12 bytes, one per aid: Lounge rank + 1, or 0 for none; u8 1 if the room
// is a mogi, a padding byte, and in a mogi each aid's LR (u16, 0: unknown), shown in place of
// VR.
const loungeRanksSize = 4 + MaxPlayerCount + 2 + MaxPlayerCount*2

func (r *Room) loungeRanksPacket() []byte {
	buf := make([]byte, loungeRanksSize)
	buf[0], buf[1], buf[2], buf[3] = 'R', 'N', 'K', 'S'
	for p, in := range r.players {
		// In mogi rooms everyone's rank shows, whatever their Ranks setting.
		if !in || p == nil || p.isBot || (!p.showLoungeRank && !r.mogi) || int(p.aid) >= MaxPlayerCount {
			continue
		}
		if rank, ok := loungeRankOf(p); ok {
			buf[4+p.aid] = byte(rank + 1)
		}
	}
	if r.mogi {
		buf[4+MaxPlayerCount] = 1
		for p, in := range r.players {
			if !in || p == nil || p.isBot || int(p.aid) >= MaxPlayerCount {
				continue
			}
			if lr, ok := loungeLROf(p); ok {
				binary.BigEndian.PutUint16(buf[4+MaxPlayerCount+2+int(p.aid)*2:], uint16(min(max(lr, 1), 0xffff)))
			}
		}
	}
	return buf
}

func (r *Room) sendLoungeRanks(p *Player) {
	if p == nil || p.isBot || p.roomManagerAddr == "" {
		return
	}
	_ = common.SendPacket(ServerName, p.connIdx, r.loungeRanksPacket())
}

func (r *Room) broadcastLoungeRanks() {
	for p, in := range r.players {
		if in {
			r.sendLoungeRanks(p)
		}
	}
}
