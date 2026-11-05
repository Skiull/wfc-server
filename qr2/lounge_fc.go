package qr2

import (
	"fmt"
	"strconv"
	"strings"

	"wwfc/common"
	"wwfc/logging"
)

// Linked Lounge profiles: MKW Lounge has each player's friend code by Discord user id
// (hostfc.php), and the ladder has each player's Discord id, so the two give a friend code
// to Lounge name table. Mogi rooms (Create Mogi) only take players found in it.


// The MKW Lounge Discord server, whose friend codes hostfc.php returns.
const loungeGuildID = "387347467332485122"

// Discord ids per hostfc.php request (300 works).
const hostFCBatch = 300

func fetchLoungeFriendCodes(entries []leaderboardEntry) (map[uint64]string, error) {
	names := map[string]string{} // Discord id: Lounge name.
	var ids []string
	for _, e := range entries {
		if e.discord != "" && names[e.discord] == "" {
			names[e.discord] = e.name
			ids = append(ids, e.discord)
		}
	}
	fcs := map[uint64]string{}
	byDiscord := map[string]uint64{}
	for start := 0; start < len(ids); start += hostFCBatch {
		batch := ids[start:min(start+hostFCBatch, len(ids))]
		var resp struct {
			Results []struct {
				Discord string `json:"discord_user_id"`
				FC      string `json:"fc"`
			}
		}
		path := fmt.Sprintf("hostfc.php?discord_guild_id=%s&discord_user_id=%s", loungeGuildID, strings.Join(batch, ","))
		if err := loungeGet(path, &resp); err != nil {
			return nil, err
		}
		for _, r := range resp.Results {
			if fc, ok := parseFriendCode(r.FC); ok {
				fcs[fc] = names[r.Discord]
				byDiscord[r.Discord] = fc
			}
		}
	}
	// Each ladder entry's friend code, and its Wiimmfi Mii (leaderboard rows).
	var pids []uint32
	for i := range entries {
		if fc := byDiscord[entries[i].discord]; fc != 0 {
			entries[i].fc = fc
			pids = append(pids, uint32(fc))
		}
	}
	miis := fetchWiimmfiMiis(pids)
	for i := range entries {
		if entries[i].fc != 0 {
			entries[i].mii = miis[uint32(entries[i].fc)]
		}
	}
	logging.Info(moduleName, "Lounge Miis from Wiimmfi:", len(miis), "of", len(pids))
	return fcs, nil
}

// parseFriendCode reads a friend code like "1234-5678-9012".
func parseFriendCode(s string) (uint64, bool) {
	digits := strings.ReplaceAll(strings.TrimSpace(s), "-", "")
	if len(digits) != 12 {
		return 0, false
	}
	fc, err := strconv.ParseUint(digits, 10, 64)
	return fc, err == nil && fc != 0
}

// loungeName is the Lounge name linked to a profile's Mario Kart Wii friend code. ok is false
// when it isn't linked, or the table hasn't loaded.
func loungeName(profileID uint32) (name string, ok bool) {
	refreshLeaderboard()
	// Mario Kart Wii friend codes use the RMCJ game id whatever the disc's region.
	fc := common.CalcFriendCode(profileID, "RMCJ")
	leaderboard.Lock()
	defer leaderboard.Unlock()
	name, ok = leaderboard.friendCodes[fc]
	return name, ok
}

func playerLoungeName(p *Player) (string, bool) {
	if p == nil || p.login == nil {
		return "", false
	}
	return loungeName(p.login.ProfileID)
}

// kickUnlinked disconnects a player trying a mogi room without a linked Lounge profile, so
// their game shows an error instead of waiting.
func kickUnlinked(p *Player) {
	if p.login != nil {
		go kickPlayer(p.login.ProfileID, "not_lounge_linked")
	}
}
