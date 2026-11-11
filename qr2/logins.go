package qr2

import (
	"encoding/gob"
	"os"
	"slices"
	"strconv"

	"wwfc/common"
)

type LoginInfo struct {
	ProfileID           uint32
	GameCode            string
	InGameName          string
	ConsoleFriendCode   uint64
	FriendKeyGame       string
	GPPublicIP          string
	NeedsExploit        bool
	DeviceAuthenticated bool
	Restricted          bool
	player              *Player

	// Must match the client's MAX_FRIEND_COUNT (64). Vanilla MKW only had 30;
	// the private client stores 0..29 in rksys.dat and 30..63 on SD.
	friendsList [64]uint32
	OpenHost    bool

	// The client's platform from login ("wl:host"), like "Dolphin ..." or "Wii".
	HostPlatform string
}

var logins = map[uint32]*LoginInfo{}

func Login(profileID uint32, gameCode string, inGameName string, consoleFriendCode uint64, fcGame string, publicIP string, needsExploit bool, deviceAuthenticated bool, restricted bool, openHost bool) {
	mutex.Lock()
	defer mutex.Unlock()

	logins[profileID] = &LoginInfo{
		ProfileID:           profileID,
		GameCode:            gameCode,
		InGameName:          inGameName,
		ConsoleFriendCode:   consoleFriendCode,
		FriendKeyGame:       fcGame,
		GPPublicIP:          publicIP,
		NeedsExploit:        needsExploit,
		DeviceAuthenticated: deviceAuthenticated,
		Restricted:          restricted,
		player:              nil,

		OpenHost: openHost,
	}
}

// SetHostPlatform records the platform a logged-in player is on.
func SetHostPlatform(profileID uint32, platform string) {
	mutex.Lock()
	defer mutex.Unlock()
	if login := logins[profileID]; login != nil {
		login.HostPlatform = platform
	}
}

func SetDeviceAuthenticated(profileID uint32) {
	mutex.Lock()
	defer mutex.Unlock()

	if login, exists := logins[profileID]; exists {
		login.DeviceAuthenticated = true
		if login.player != nil {
			login.player.Data["+deviceauth"] = "1"
		}
	}
}

func Logout(profileID uint32) {
	mutex.Lock()
	defer mutex.Unlock()

	// Delete login's player
	if login, exists := logins[profileID]; exists {
		if login.player != nil {
			removePlayer(common.MakeLookupAddr(login.player.Addr.String()))
		}
	}

	delete(logins, profileID)
}

// Save logins to a file. Expects the mutex to be locked.
func saveLogins() error {
	file, err := os.OpenFile("state/qr2_logins.gob", os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		return err
	}

	encoder := gob.NewEncoder(file)
	err = encoder.Encode(logins)
	file.Close()
	return err
}

// Load logins from a file. Expects the mutex to be locked, and the players to already be loaded.
func loadLogins() error {
	file, err := os.Open("state/qr2_logins.gob")
	if err != nil {
		return err
	}

	decoder := gob.NewDecoder(file)
	err = decoder.Decode(&logins)
	file.Close()
	if err != nil {
		return err
	}

	for _, player := range players {
		dwcPid := player.Data["dwc_pid"]
		if dwcPid == "" {
			continue
		}

		profileID, err := strconv.ParseUint(dwcPid, 10, 32)
		if err != nil {
			continue
		}

		if login, exists := logins[uint32(profileID)]; exists {
			login.player = player
			player.login = login
		}
	}

	return nil
}

func AddToFriendsList(profileId uint32, friendId uint32) {
	mutex.Lock()
	defer mutex.Unlock()
	if login, exists := logins[profileId]; exists {
		if i := slices.Index(login.friendsList[:], uint32(0)); i != -1 {
			login.friendsList[i] = friendId
		}
	}
}

func RemoveFromFriendList(profileId uint32, friendId uint32) {
	mutex.Lock()
	defer mutex.Unlock()
	if login, exists := logins[profileId]; exists {
		if i := slices.Index(login.friendsList[:], friendId); i != -1 {
			login.friendsList[i] = 0
		}
	}
}
