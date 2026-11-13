package nas

import "testing"

func TestFromLoopback(t *testing.T) {
	for addr, want := range map[string]bool{
		"127.0.0.1:5000":    true,
		"[::1]:5000":        true,
		"192.168.1.50:5000": false,
		"198.51.100.7:3328": false,
		"garbage":           false,
	} {
		if got := fromLoopback(addr); got != want {
			t.Errorf("fromLoopback(%q) = %v, want %v", addr, got, want)
		}
	}
}

func TestFromLocalNetwork(t *testing.T) {
	for addr, want := range map[string]bool{
		"127.0.0.1:5000":    true,
		"192.168.0.1:1024":  true,
		"192.168.1.50:5000": true,
		"198.51.100.7:3328": false,
		"203.0.113.5:80":    false,
	} {
		if got := fromLocalNetwork(addr); got != want {
			t.Errorf("fromLocalNetwork(%q) = %v, want %v", addr, got, want)
		}
	}
}

func TestIsGameHost(t *testing.T) {
	for host, want := range map[string]bool{
		"cb007105.nip.io":           true,
		"cb007105.nip.io:80":        true,
		"nas.nwfc.wiinoma.com":      true,
		"conntest.nintendowifi.net": true,
		"203.0.113.5":               false,
		"203.0.113.5:80":            false,
		"detectportal.firefox.com":  false,
		"checkip.amazonaws.com":     false,
	} {
		if got := isGameHost(host); got != want {
			t.Errorf("isGameHost(%q) = %v, want %v", host, got, want)
		}
	}
}
