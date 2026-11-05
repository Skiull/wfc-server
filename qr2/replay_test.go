package qr2

import (
	"os"
	"path/filepath"
	"testing"
)

func TestChooseReplayOwnRacesNewestFirst(t *testing.T) {
	dir := t.TempDir()
	day := filepath.Join(dir, "2026-11-01")
	os.MkdirAll(day, 0o755)
	write := func(name, players string) {
		base := filepath.Join(day, name)
		os.WriteFile(base+".pkt", []byte("x"), 0o644)
		os.WriteFile(base+".players.json", []byte(players), 0o644)
	}
	write("100000_course8_1", `{"course":8,"players":[0,1],"profiles":{"0":7,"1":8}}`)
	write("110000_course2_1", `{"course":2,"players":[0],"profiles":{"0":9}}`)
	twelve := `{"course":11,"players":[0,1,2,3,4,5,6,7,8,9,10,11],"profiles":{"0":1,"1":2,"2":3,"3":4,"4":5,"5":7,"6":10,"7":11,"8":12,"9":13,"10":14,"11":15}}`
	write("120000_course11_1", twelve)

	list := ownReplays(dir, 7)
	if len(list) != 2 || list[0].course != 11 || list[1].course != 8 {
		t.Fatalf("%+v", list)
	}
	c, err := chooseReplay(dir, 7, 0)
	if err != nil || filepath.Base(c.file) != "120000_course11_1.pkt" || c.bots != 11 || !c.guest {
		t.Fatalf("%+v %v", c, err)
	}
	c, err = chooseReplay(dir, 7, 1)
	if err != nil || c.bots != 2 || c.guest {
		t.Fatalf("%+v %v", c, err)
	}
	if _, err := chooseReplay(dir, 7, 2); err == nil {
		t.Fatal("index past the list")
	}
	pkt := packReplayList(list)
	if pkt[4] != 2 || pkt[8] != 12 || pkt[11] != 'M' { // "Maple Treeway", UTF-16BE.
		t.Fatalf("%x", pkt[:16])
	}
}

func TestCourseNamesFollowRaceInfoIds(t *testing.T) {
	// A DK Mountain race was recorded as course 19.
	if courseNames[19] != "GCN DK Mountain" || courseNames[8] != "Luigi Circuit" || len(courseNames) != 32 {
		t.Fatal(courseNames[19], courseNames[8], len(courseNames))
	}
}
