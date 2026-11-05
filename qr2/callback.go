package qr2

var gpErrorCallback func(uint32, string)

func SetGPErrorCallback(callback func(uint32, string)) {
	gpErrorCallback = callback
}

// Kicks a player for misbehaving, also telling their room's mkw-server to stop relaying
// them right away. Expects the mutex to be unlocked.
func kickPlayer(profileID uint32, reason string) {
	KickFromMKWServer(profileID)
	gpErrorCallback(profileID, reason)
}
