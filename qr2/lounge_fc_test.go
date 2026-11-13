package qr2

import (
	"encoding/xml"
	"testing"

	"wwfc/common"
)

func TestParseFriendCode(t *testing.T) {
	if fc, ok := parseFriendCode("1234-5678-9012"); !ok || fc != 123456789012 {
		t.Fatal(fc, ok)
	}
	if _, ok := parseFriendCode("12-34"); ok {
		t.Fatal("short code accepted")
	}
}

func TestLoungeNameByProfile(t *testing.T) {
	leaderboard.Lock()
	leaderboard.friendCodes = map[uint64]string{common.CalcFriendCode(1000000000, "RMCJ"): "Skull"}
	leaderboard.fetchedAt = leaderboard.fetchedAt.AddDate(100, 0, 0) // Not stale: no fetch.
	leaderboard.Unlock()
	defer func() { leaderboard.friendCodes = nil }()
	if name, ok := loungeName(1000000000); !ok || name != "Skull" {
		t.Fatal(name, ok)
	}
	if _, ok := loungeName(1234); ok {
		t.Fatal("unlinked profile found")
	}
}

func TestParseSakeMiis(t *testing.T) {
	const reply = `<?xml version="1.0" encoding="utf-8"?><soap:Envelope xmlns:soap="http://schemas.xmlsoap.org/soap/envelope/"><soap:Body><SearchForRecordsResponse xmlns="http://gamespy.net/sake"><SearchForRecordsResult>Success</SearchForRecordsResult><values><ArrayOfRecordValue><RecordValue><binaryDataValue><value>wA4AVgBBACAATgBhAG4AYQBzAGUAAEAsiYdNAkLMUgUgQCjAKv0MMiBwDoAAaZgPMO8AiiUEAAAAAAAAAAAAAAAAAAAAAAAAAABHhgAS2jgyzqr1Uk1DUAAAAABpb24u</value></binaryDataValue></RecordValue><RecordValue><intValue><value>390148870</value></intValue></RecordValue></ArrayOfRecordValue></values></SearchForRecordsResponse></soap:Body></soap:Envelope>`
	var parsed sakeResponse
	if err := xml.Unmarshal([]byte(reply), &parsed); err != nil {
		t.Fatal(err)
	}
	miis := parseSakeMiis(parsed)
	mii := miis[390148870]
	if len(mii) != rawMiiLen || mii[3] != 'V' {
		t.Fatalf("%d bytes: %x", len(mii), mii)
	}
}
