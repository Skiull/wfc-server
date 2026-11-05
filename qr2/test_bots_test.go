package qr2

import (
	"io"
	"net"
	"testing"

	"wwfc/common"
)

func roomWithBots(t *testing.T, bots int) (*Room, *Player) {
	t.Helper()
	host := &Player{PlayerId: 1, Addr: net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 5000}, localPlayerCount: 1}
	r := &Room{
		roomName:       "test",
		Region:         common.WorldWide,
		host:           host,
		players:        map[*Player]bool{host: true},
		waitingPlayers: map[*Player]bool{},
		mkwServer:      &MKWServer{},
	}
	if err := r.tryAddPlayer(host, true); err != nil {
		t.Fatal(err)
	}
	r.addTestBots(bots)
	return r, host
}

func TestAddTestBotsFillsRoom(t *testing.T) {
	r, host := roomWithBots(t, 11)

	if !r.full() || r.numAids != 12 || r.aidBitmap != 0xfff {
		t.Fatalf("room not full: numAids %d, bitmap %#x", r.numAids, r.aidBitmap)
	}
	if host.aid != 0 {
		t.Fatalf("host aid %d", host.aid)
	}
	for p := range r.players {
		if p != host && (!p.isBot || p.roomPointer != r) {
			t.Fatalf("player %+v isn't a bot in the room", p)
		}
	}

	// Asking for more bots than there's room for stops at full.
	r2, _ := roomWithBots(t, 20)
	if r2.numPlayers() != MaxPlayerCount {
		t.Fatalf("got %d players", r2.numPlayers())
	}
}

func TestBotsNeverHost(t *testing.T) {
	r, host := roomWithBots(t, 3)
	r.aidBitmap = clearAid(r.aidBitmap, host.aid)
	if err := r.migrateHost(); err == nil {
		t.Fatalf("migrated host to bot aid %d", r.host.aid)
	}
}

func TestRoomClosesWithOnlyBots(t *testing.T) {
	r, host := roomWithBots(t, 5)
	if !r.shouldCloseRoom(host) {
		t.Fatal("room stays open when only bots would be left")
	}

	other := &Player{PlayerId: 2, localPlayerCount: 1}
	r.players[other] = true
	if r.shouldCloseRoom(host) {
		t.Fatal("room closes while a real player is left")
	}
}

func TestBotsDontBlockSuspension(t *testing.T) {
	r, host := roomWithBots(t, 5)
	host.suspendVote = true
	r.updateSuspension()
	if !r.suspended {
		t.Fatal("room didn't suspend when the only real player voted to")
	}
}

func TestBotsRegisteredWithMKWServer(t *testing.T) {
	r, _ := roomWithBots(t, 2)

	client, server := net.Pipe()
	defer client.Close()
	defer server.Close()
	r.mkwServer.conn = client

	done := make(chan []byte)
	go func() {
		buf := make([]byte, 22*3) // Type byte + 21 byte AddPlayer layout.
		io.ReadFull(server, buf)
		done <- buf
	}()

	if err := r.sendMKWServerJoinRoomForEachPlayer(); err != nil {
		t.Fatal(err)
	}

	buf := <-done
	types := map[byte]int{}
	for i := 0; i < len(buf); i += 22 {
		types[buf[i]]++
	}
	if types[AddPlayer] != 1 || types[AddBot] != 2 {
		t.Fatalf("got request types %v, want 1 AddPlayer and 2 AddBot", types)
	}
}
