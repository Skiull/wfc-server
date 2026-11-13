package common

import (
	"os"
	"testing"
	"time"
)

func TestConfigReload(t *testing.T) {
	dir := t.TempDir()
	old, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(old)
	configLoaded, configCheckInterval = false, 0
	defer func() { configLoaded, configCheckInterval = false, 2*time.Second }()

	write := func(body string, mod time.Time) {
		os.WriteFile("config.xml", []byte(body), 0o644)
		os.Chtimes("config.xml", mod, mod)
	}
	t0 := time.Now().Add(-time.Hour)
	write("<Config><loginGameHashes>aa</loginGameHashes></Config>", t0)
	if got := GetConfig().LoginGameHashes; got != "aa" {
		t.Fatalf("got %q", got)
	}
	write("<Config><loginGameHashes>aa,bb</loginGameHashes></Config>", t0.Add(time.Minute))
	if got := GetConfig().LoginGameHashes; got != "aa,bb" {
		t.Fatalf("change not reloaded: %q", got)
	}
	write("<Config><loginGameHashes>broken", t0.Add(2*time.Minute))
	if got := GetConfig().LoginGameHashes; got != "aa,bb" {
		t.Fatalf("broken file replaced the settings: %q", got)
	}
}
