package qr2

import (
	"fmt"

	"wwfc/common"
	"wwfc/logging"
)

func findPublicRoom(p *Player, region common.MKWServerSearchRegion, gameMode common.MKWServerGameMode) error {
	// Replay rooms are for one watcher: never join someone else's.
	if region == common.Replay {
		return createRoom(p, region, gameMode)
	}

	for id, r := range rooms {
		if id == "" || r == nil {
			continue
		}

		if r.Region == common.Private {
			continue
		}

		// check that the rooms region is the same as the requested region
		if r.Region != region {
			continue
		}

		if r.gameMode != gameMode {
			continue
		}

		// First try to add player to the room
		err := r.tryAddPlayer(p, false)
		if err != nil {
			logging.Info(moduleName, "findPublicRoom() err:", err.Error(), "Will attempt to add player to wait list")
		} else {
			logging.Info(moduleName, "Successfully added Player", p.PlayerId, "to room", r.roomName)
			return nil
		}

		// Then try to add player to the room's wait list
		err = r.tryAddWaitingPlayer(p)
		if err != nil {
			logging.Info(moduleName, "findPublicRoom(): Couldn't add player", p.PlayerId, "to room", r.roomName, "wait list for reason", err.Error())
		} else {
			logging.Info(moduleName, "Successfully added Player", p.PlayerId, "to room", r.roomName, "wait list")
			return nil
		}
	}

	// if we reach here, then there are no available rooms, so create a public room with the specified region
	logging.Info(moduleName, "No rooms specify search criteria, creating a room for Player", p.PlayerId)
	err := createRoom(p, region, gameMode)
	if err != nil {
		return fmt.Errorf("Player %d failed to create a public room. createRoom() returned %s", p.PlayerId, err.Error())
	}
	return nil

}
