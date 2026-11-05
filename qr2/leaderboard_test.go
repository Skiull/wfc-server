package qr2

import (
	"encoding/binary"
	"testing"
)

func TestRankForLR(t *testing.T) {
	for lr, want := range map[int]string{0: "iron", 1249: "iron", 1250: "copper", 5500: "gold", 11755: "diamond", 12500: "pearl", 13000: "master", 20000: "grandmaster"} {
		if got := leaderboardRanks[rankForLR(lr)].name; got != want {
			t.Errorf("LR %d: %s, want %s", lr, got, want)
		}
	}
}

func TestPackLeaderboardPage(t *testing.T) {
	leaderboard.entries = nil
	for i := 0; i < 23; i++ {
		leaderboard.entries = append(leaderboard.entries, leaderboardEntry{name: "Player", country: "US", lr: 11755 - i})
	}
	defer func() { leaderboard.entries = nil }()
	buf := packLeaderboardPage(2)
	if len(buf) != 8+10*leaderboardRowSize {
		t.Fatalf("size %d", len(buf))
	}
	if page, pages := binary.BigEndian.Uint16(buf[4:]), binary.BigEndian.Uint16(buf[6:]); page != 2 || pages != 3 {
		t.Fatalf("page %d of %d", page, pages)
	}
	row := buf[8:]
	if pos := binary.BigEndian.Uint32(row); pos != 21 || row[10] != 49 || row[8] != 0xf2 {
		t.Fatalf("row %x", row[:leaderboardRowSize])
	}
	if pos := binary.BigEndian.Uint32(buf[8+3*leaderboardRowSize:]); pos != 0 {
		t.Fatalf("row 4 should be empty, position %d", pos)
	}
}
