package qr2

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"unicode/utf16"

	"wwfc/common"
)

// The Replays menu: the newest packet recordings of races the player was in, newest first.
// The client shows them as a list and searches region common.Replay with the chosen index.

const (
	maxReplayChoices      = 12
	ReplayListPacketMagic = 0x52504C53 // 'RPLS'
	replayLabelLen        = 24         // UTF-16 code units per text field.
)

// Course names by the course id in RaceInfo records (mkw-server common.Course order).
var courseNames = []string{
	"Mario Circuit", "Moo Moo Meadows", "Mushroom Gorge", "Grumble Volcano", "Toad's Factory",
	"Coconut Mall", "DK Summit", "Wario's Gold Mine", "Luigi Circuit", "Daisy Circuit",
	"Moonview Highway", "Maple Treeway", "Bowser's Castle", "Rainbow Road", "Dry Dry Ruins",
	"Koopa Cape", "GCN Peach Beach", "GCN Mario Circuit", "GCN Waluigi Stadium", "GCN DK Mountain",
	"DS Yoshi Falls", "DS Desert Hills", "DS Peach Gardens", "DS Delfino Square",
	"SNES Mario Circuit 3", "SNES Ghost Valley 2", "N64 Mario Raceway", "N64 Sherbet Land",
	"N64 Bowser's Castle", "N64 DK's Jungle Parkway", "GBA Bowser Castle 3", "GBA Shy Guy Beach",
}

type replayEntry struct {
	file    string
	at      time.Time
	course  int
	players int
	profile map[string]uint32
}

// ownReplays lists the newest recordings of races profileID was in, newest first.
func ownReplays(dir string, profileID uint32) []replayEntry {
	var list []replayEntry
	if dir == "" {
		return nil
	}
	filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".pkt") {
			return nil
		}
		data, err := os.ReadFile(strings.TrimSuffix(path, ".pkt") + ".players.json")
		if err != nil {
			return nil
		}
		var meta struct {
			Course   int
			Players  []int
			Profiles map[string]uint32
		}
		if json.Unmarshal(data, &meta) != nil || len(meta.Players) == 0 {
			return nil
		}
		raced, real := false, 0
		for _, p := range meta.Profiles {
			raced = raced || (p != 0 && p == profileID)
			if p != 0 {
				real++ // Test bots (profile 0) aren't shown.
			}
		}
		if !raced || real == 0 {
			return nil
		}
		at := recordingTime(path)
		list = append(list, replayEntry{file: path, at: at, course: meta.Course, players: real})
		return nil
	})
	sort.Slice(list, func(i, j int) bool { return list[i].at.After(list[j].at) })
	if len(list) > maxReplayChoices {
		list = list[:maxReplayChoices]
	}
	return list
}

// recordingTime reads a recording's start from its path (<date>/<hhmmss>_...), falling back
// to the file time.
func recordingTime(path string) time.Time {
	day := filepath.Base(filepath.Dir(path))
	name := filepath.Base(path)
	if len(name) >= 6 {
		if t, err := time.ParseInLocation("2006-01-02 150405", day+" "+name[:6], time.Local); err == nil {
			return t
		}
	}
	if info, err := os.Stat(path); err == nil {
		return info.ModTime()
	}
	return time.Time{}
}

// Packet: magic, u8 count, 3 bytes padding, then 12 entries of
//
//	u8 players, u8 padding, u16 course[24] (name, UTF-16BE), u16 when[24] ("Nov 1 21:04")
const replayEntrySize = 2 + replayLabelLen*4

func packReplayList(list []replayEntry) []byte {
	buf := make([]byte, 8+maxReplayChoices*replayEntrySize)
	binary.BigEndian.PutUint32(buf[0:], ReplayListPacketMagic)
	buf[4] = byte(len(list))
	for i, e := range list {
		row := buf[8+i*replayEntrySize:]
		row[0] = byte(e.players)
		name := fmt.Sprintf("Course %d", e.course)
		if e.course >= 0 && e.course < len(courseNames) {
			name = courseNames[e.course]
		}
		putUTF16(row[2:], name)
		putUTF16(row[2+replayLabelLen*2:], e.at.Format("Jan 2 15:04"))
	}
	return buf
}

func putUTF16(dst []byte, s string) {
	u := utf16.Encode([]rune(s))
	for j := 0; j < len(u) && j < replayLabelLen; j++ {
		binary.BigEndian.PutUint16(dst[j*2:], u[j])
	}
}

func handleReplayListRequest(p *Player) error {
	var profileID uint32
	if p.login != nil {
		profileID = p.login.ProfileID
	}
	return common.SendPacket(ServerName, p.connIdx, packReplayList(ownReplays(common.GetConfig().ReplayDir, profileID)))
}
