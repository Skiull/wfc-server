package common

type MKWServerSearchRegion int
type MKWServerGameMode int

// Search Regions clients can search for. Custom regions can be added here.
const (
	Private   = 0
	WorldWide = 1
	NA        = 2
	EU        = 3
	JP        = 4
	KOR       = 5
	// Not a real region: the Replays button. Each search gets its own room that replays a
	// recorded race (see qr2/replay.go).
	Replay = 6
)

// Game mode clients can search for
const (
	Undecided = 0
	VS        = 1
	Battle    = 2
)
