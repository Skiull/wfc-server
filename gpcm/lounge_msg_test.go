package gpcm

import (
	"fmt"
	"strings"
	"testing"
)

func TestNotLoungeLinkedMessage(t *testing.T) {
	msg := fmt.Sprintf(WWFCMsgNotLoungeLinked.MessageRMC[LangEnglish], WWFCMsgNotLoungeLinked.ErrorCode)
	if !strings.Contains(msg, "This friend code is not registered\nin Lounge.") {
		t.Fatal(msg)
	}
}
