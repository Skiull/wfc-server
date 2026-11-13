package gpcm

import (
	"time"
	"wwfc/common"
	"wwfc/database"
	"wwfc/logging"
	"wwfc/qr2"

	"github.com/logrusorgru/aurora/v3"
)

func kickPlayer(profileID uint32, reason string) {
	if session, exists := sessions[profileID]; exists {
		errorMessage := WWFCMsgKickedGeneric

		switch reason {
		case "banned":
			errorMessage = WWFCMsgProfileBannedTOSNow

		case "restricted":
			errorMessage = WWFCMsgProfileRestrictedNow

		case "restricted_join":
			errorMessage = WWFCMsgProfileRestricted

		case "moderator_kick":
			errorMessage = WWFCMsgKickedModerator

		case "room_kick":
			errorMessage = WWFCMsgKickedRoomHost

		case "cheating":
			errorMessage = WWFCMsgKickedCheating

		case "bad_version":
			errorMessage = WWFCMsgBadGameVersion

		case "not_lounge_linked":
			errorMessage = WWFCMsgNotLoungeLinked

		case "invalid_elo":
			errorMessage = WWFCMsgInvalidELO

		case "network_error":
			// No error message
			common.CloseConnection(ServerName, session.ConnIndex)
			return
		}

		session.replyError(GPError{
			ErrorCode:   ErrConnectionClosed.ErrorCode,
			ErrorString: "The player was kicked from the server. Reason: " + reason,
			Fatal:       true,
			WWFCMessage: errorMessage,
		})
	}
}

func KickPlayer(profileID uint32, reason string) {
	mutex.Lock()
	defer mutex.Unlock()

	kickPlayer(profileID, reason)
	banCheater(profileID, reason)
}

// banCheater bans a player mkw-server kicked for cheating, so they can't just log back in.
// The ban shows as "banned from the service" at login; lift it with /api/unban (unban.ps1).
func banCheater(profileID uint32, reason string) {
	hours := common.GetConfig().CheatBanHours
	if reason != "cheating" || hours <= 0 {
		return
	}
	if database.BanUser(pool, ctx, profileID, true, time.Duration(hours)*time.Hour, "Kicked for cheating", "mkw-server anti-cheat kick", "mkw-server") {
		logging.Notice("GPCM", "Banned profile", aurora.Cyan(profileID), "for", aurora.Cyan(hours), "hours for cheating")
	} else {
		logging.Error("GPCM", "Failed to ban profile", aurora.Cyan(profileID), "for cheating")
	}
}

func KickPlayerCustomMessage(profileID uint32, reason string, message WWFCErrorMessage) {
	// Stop mkw-server relaying the player now. Done before taking the gpcm mutex since
	// qr2 can call into gpcm while holding its own mutex.
	qr2.KickFromMKWServer(profileID)

	mutex.Lock()
	defer mutex.Unlock()

	if session, exists := sessions[profileID]; exists {
		session.replyError(GPError{
			ErrorCode:   ErrConnectionClosed.ErrorCode,
			ErrorString: "The player was kicked from the server. Reason: " + reason,
			Fatal:       true,
			WWFCMessage: message,
			Reason:      reason,
		})
	}
}
