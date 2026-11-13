package gpcm

import (
	"fmt"
	"testing"
	"unicode/utf16"
)

func TestCheatingMessage(t *testing.T) {
	msg := fmt.Sprintf(WWFCMsgKickedCheating.MessageRMC[LangEnglish], WWFCMsgKickedCheating.ErrorCode, 0x1234, "cheating")
	want := "\x1a\u0800\u0001\u0001You have been kicked for cheating.\n\n\x1a\u0800\u0001\u0000Error Code: 00000"
	if msg != want {
		t.Fatalf("got %q", msg)
	}
	units := utf16.Encode([]rune(msg))
	if units[0] != 0x1a || units[1] != 0x0800 || units[2] != 0x0001 || units[3] != 0x0001 {
		t.Fatalf("color escape encodes as %04x", units[:4])
	}
}

func TestBadVersionMessage(t *testing.T) {
	msg := fmt.Sprintf(WWFCMsgBadGameVersion.MessageRMC[LangEnglish], WWFCMsgBadGameVersion.ErrorCode, 0x1234, "bad_version")
	if msg != "Invalid game version, please update.\n\nError Code: 22004" {
		t.Fatalf("got %q", msg)
	}
}
