package qr2

import (
	"errors"
	"fmt"
)

// Replay rooms (the in-game Replays button, search region common.Replay): mkw-server plays a
// packet recording back with one bot per recorded racer, for a single watcher.

type replayChoice struct {
	file string // .pkt recording passed to mkw-server's -replay.
	bots int    // One per recorded racer, up to 11 (the watcher takes the 12th aid).
	// A 12-player race: the first bot also carries the watcher's own recorded player as a
	// guest (mkw-server does the same on its side).
	guest bool
}

// chooseReplay picks the player's index-th newest recording (see ownReplays).
func chooseReplay(dir string, profileID uint32, index int) (*replayChoice, error) {
	if dir == "" {
		return nil, errors.New("replayDir isn't set in the config")
	}
	list := ownReplays(dir, profileID)
	if index < 0 || index >= len(list) {
		return nil, fmt.Errorf("player %d has no replay %d (%d recorded)", profileID, index, len(list))
	}
	e := list[index]
	choice := &replayChoice{file: e.file, bots: min(e.players, MaxPlayerCount-1)}
	choice.guest = e.players > choice.bots
	return choice, nil
}
