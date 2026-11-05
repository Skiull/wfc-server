package qr2

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode/utf16"

	"wwfc/common"
	"wwfc/logging"
)

// The in-game leaderboard (client: Leaderboard button on the online menu). It shows the MKW
// Lounge Regular Track ladder, 10 players per page: position, name, country, LR and rank.
// Rank cutoffs and colors live here so they can change without a client update.

const (
	loungeAPI             = "https://www.mkwlounge.gg/api/"
	leaderboardRefresh    = 15 * time.Minute
	leaderboardPageSize   = 10
	leaderboardNameLen    = 16 // UTF-16 code units, no terminator needed.
	LeaderboardPacketMagic = 0x4C444244 // 'LDBD'
)

// leaderboardRank is a Lounge rank: the lowest LR that has it, and its LR color. Cutoffs from
// the Lounge rank list; colors from mkwlounge.gg's ladder (season 17). Nobody is Master or
// Grandmaster yet; their colors are from the Lounge rank list.
type leaderboardRank struct {
	minLR int
	name  string
	color uint32
}

// Index order matches the client's rank icons.
var leaderboardRanks = []leaderboardRank{
	{0, "iron", 0x817876},
	{1250, "copper", 0xcf6853},
	{2500, "bronze", 0xcd7f32},
	{4000, "silver", 0xb7bec4},
	{5500, "gold", 0xffd700},
	{7000, "platinum", 0x3fabb8},
	{8250, "emerald", 0x87f19b},
	{9500, "ruby", 0xf1383a},
	{11000, "diamond", 0xb9f2ff},
	{12000, "pearl", 0xe5b3c7},
	{13000, "master", 0xcfe2f3},
	{14000, "grandmaster", 0x971717},
}

func rankForLR(lr int) int {
	r := 0
	for i, rank := range leaderboardRanks {
		if lr >= rank.minLR {
			r = i
		}
	}
	return r
}

type leaderboardEntry struct {
	name    string
	country string
	lr      int
	discord string // Discord user id, to find the player's friend code (lounge_fc.go).
	pad     byte   // Controller (see loungeControllers), 0 if not set.
	fc      uint64 // Friend code, if Lounge has one (lounge_fc.go).
	mii     []byte // Raw Mii from Wiimmfi (wiimmfi_mii.go), if any.
}

// Controllers as Lounge names them, and their number in the leaderboard packet (the client
// shows the game's picture of each).
var loungeControllers = map[string]byte{
	"wiiwheel": 1, "nunchuk": 2, "classic": 3, "gcn": 4, "classicpro": 5, "ccp": 5,
}

var leaderboard struct {
	sync.Mutex
	entries     []leaderboardEntry
	friendCodes map[uint64]string // Friend code: Lounge name.
	fetchedAt   time.Time
	fetching  bool
}

// refreshLeaderboard fetches the ladder if it's stale, in the background.
func refreshLeaderboard() {
	leaderboard.Lock()
	stale := time.Since(leaderboard.fetchedAt) > leaderboardRefresh
	if !stale || leaderboard.fetching {
		leaderboard.Unlock()
		return
	}
	leaderboard.fetching = true
	leaderboard.Unlock()

	go func() {
		entries, err := fetchLoungeLadder()
		var fcs map[uint64]string
		if err == nil {
			fcs, err = fetchLoungeFriendCodes(entries)
		}
		leaderboard.Lock()
		defer leaderboard.Unlock()
		leaderboard.fetching = false
		if err != nil {
			logging.Error(moduleName, "Leaderboard fetch failed:", err.Error())
			leaderboard.fetchedAt = time.Now().Add(-leaderboardRefresh + time.Minute) // Retry in a minute.
			return
		}
		leaderboard.entries = entries
		leaderboard.friendCodes = fcs
		leaderboard.fetchedAt = time.Now()
		logging.Info(moduleName, "Leaderboard updated:", len(entries), "players,", len(fcs), "with friend codes")
	}()
}

var loungeClient = &http.Client{Timeout: 20 * time.Second}

func loungeGet(path string, v any) error {
	resp, err := loungeClient.Get(loungeAPI + path)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("%s: %s", path, resp.Status)
	}
	return json.NewDecoder(resp.Body).Decode(v)
}

func fetchLoungeLadder() ([]leaderboardEntry, error) {
	var ladders struct {
		Results []struct {
			LadderID int `json:"ladder_id"`
			IsActive int `json:"is_active"`
		}
	}
	if err := loungeGet("ladder.php?ladder_type=rt", &ladders); err != nil {
		return nil, err
	}
	ladderID := 0
	for _, l := range ladders.Results {
		if l.IsActive == 1 || ladderID == 0 {
			ladderID = l.LadderID
		}
	}
	if ladderID == 0 {
		return nil, fmt.Errorf("no RT ladder")
	}
	var players struct {
		Results []struct {
			Name    string `json:"player_name"`
			Country string `json:"player_country_flag"`
			LR      int    `json:"current_lr"`
			Discord string `json:"discord_user_id"`
			Pad     string `json:"game_controller"`
		}
	}
	if err := loungeGet(fmt.Sprintf("ladderplayer.php?ladder_id=%d&all=1", ladderID), &players); err != nil {
		return nil, err
	}
	entries := make([]leaderboardEntry, 0, len(players.Results))
	for _, p := range players.Results {
		entries = append(entries, leaderboardEntry{name: p.Name, country: strings.ToUpper(p.Country), lr: p.LR, discord: p.Discord, pad: loungeControllers[p.Pad]})
	}
	sort.SliceStable(entries, func(i, j int) bool { return entries[i].lr > entries[j].lr })
	return entries, nil
}

// Packet: magic, u16 page, u16 page count, then 10 rows of
//
//	u32 position (0: empty row), u16 LR, u8 rank, u8 r, u8 g, u8 b,
//	u8 Wii country code of the flag (0: none, see leaderboard_flags.go),
//	u8 controller (0: none, 1 Wii Wheel, 2 Nunchuk, 3 Classic, 4 GameCube, 5 Classic Pro),
//	u16 name[16] (UTF-16BE, zero padded),
//	u8 mii[0x4c] (raw Mii; all zero: none)
//
// = 4 + 4 + 10*120 bytes.
const leaderboardRowSize = 4 + 2 + 1 + 3 + 2 + leaderboardNameLen*2 + rawMiiLen

func packLeaderboardPage(page int) []byte {
	leaderboard.Lock()
	entries := leaderboard.entries
	leaderboard.Unlock()

	pages := max(1, (len(entries)+leaderboardPageSize-1)/leaderboardPageSize)
	page = min(max(page, 0), pages-1)
	buf := make([]byte, 8+leaderboardPageSize*leaderboardRowSize)
	binary.BigEndian.PutUint32(buf[0:], LeaderboardPacketMagic)
	binary.BigEndian.PutUint16(buf[4:], uint16(page))
	binary.BigEndian.PutUint16(buf[6:], uint16(pages))
	for i := 0; i < leaderboardPageSize; i++ {
		n := page*leaderboardPageSize + i
		if n >= len(entries) {
			break
		}
		e := entries[n]
		row := buf[8+i*leaderboardRowSize:]
		rank := rankForLR(e.lr)
		c := leaderboardRanks[rank].color
		binary.BigEndian.PutUint32(row[0:], uint32(n+1))
		binary.BigEndian.PutUint16(row[4:], uint16(min(max(e.lr, 0), 0xffff)))
		row[6], row[7], row[8], row[9] = byte(rank), byte(c>>16), byte(c>>8), byte(c)
		row[10], row[11] = byte(wiiFlagCountries[e.country]), e.pad
		copy(row[12+leaderboardNameLen*2:], e.mii)
		name := utf16.Encode([]rune(e.name))
		for j := 0; j < len(name) && j < leaderboardNameLen; j++ {
			binary.BigEndian.PutUint16(row[12+j*2:], name[j])
		}
	}
	return buf
}

func handleLeaderboardRequest(p *Player, page int) error {
	refreshLeaderboard()
	return common.SendPacket(ServerName, p.connIdx, packLeaderboardPage(page))
}
