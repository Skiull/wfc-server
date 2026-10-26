package qr2

import (
	"bytes"
	"net"
	"testing"
)

func TestPackKickPlayerRequest(t *testing.T) {
	p := &Player{Addr: net.UDPAddr{IP: net.IPv4(10, 1, 2, 3), Port: 0x1234}}
	got, err := packKickPlayerRequest(p)
	if err != nil {
		t.Fatal(err)
	}
	want := []byte{KickAid, 10, 1, 2, 3, 0x12, 0x34}
	if !bytes.Equal(got, want) {
		t.Fatalf("got % x, want % x", got, want)
	}
}

func TestUnpackAidKickedResponse(t *testing.T) {
	msg := []byte{AidKicked, 10, 1, 2, 3, 0x12, 0x34, 5, 1}
	r, err := unpackAidKickedResponse(msg)
	if err != nil {
		t.Fatal(err)
	}
	if r.ip != 0x0a010203 || r.port != 0x1234 || r.aid != 5 || r.reason != KickReasonInvalidPackets {
		t.Fatalf("unexpected response %+v", r)
	}
	if _, err := unpackAidKickedResponse(msg[:8]); err == nil {
		t.Fatal("expected error for short message")
	}
}

func TestFindPlayerByAddrAndAid(t *testing.T) {
	p := &Player{Addr: net.UDPAddr{IP: net.IPv4(10, 1, 2, 3), Port: 0x1234}, aid: 5}
	r := &Room{players: map[*Player]bool{p: true}}
	if r.findPlayerByAddrAndAid(0x0a010203, 0x1234, 5) != p {
		t.Fatal("expected to find player")
	}
	if r.findPlayerByAddrAndAid(0x0a010203, 0x1234, 6) != nil {
		t.Fatal("aid mismatch should not match")
	}
	if r.findPlayerByAddrAndAid(0x0a010204, 0x1234, 5) != nil {
		t.Fatal("ip mismatch should not match")
	}
}
