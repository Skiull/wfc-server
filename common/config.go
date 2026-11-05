package common

import (
	"encoding/xml"
	"fmt"
	"os"
	"sync"
	"time"
)

type Config struct {
	Username        string `xml:"username"`
	Password        string `xml:"password"`
	DatabaseAddress string `xml:"databaseAddress"`
	DatabaseName    string `xml:"databaseName"`

	DefaultAddress  string  `xml:"address"`
	GameSpyAddress  *string `xml:"gsAddress,omitempty"`
	NASAddress      *string `xml:"nasAddress,omitempty"`
	NASPort         string  `xml:"nasPort"`
	NASAddressHTTPS *string `xml:"nasAddressHttps,omitempty"`
	NASPortHTTPS    string  `xml:"nasPortHttps"`

	FrontendAddress        string `xml:"frontendAddress"`
	FrontendBackendAddress string `xml:"frontendBackendAddress"`
	BackendAddress         string `xml:"backendAddress"`
	BackendFrontendAddress string `xml:"backendFrontendAddress"`

	EnableHTTPS           bool  `xml:"enableHttps"`
	EnableHTTPSExploitWii *bool `xml:"enableHttpsExploitWii,omitempty"`
	EnableHTTPSExploitDS  *bool `xml:"enableHttpsExploitDS,omitempty"`

	LogLevel  *int   `xml:"logLevel"`
	LogOutput string `xml:"logOutput"`

	CertPath      string `xml:"certPath"`
	KeyPath       string `xml:"keyPath"`
	CertPathWii   string `xml:"certDerPathWii"`
	KeyPathWii    string `xml:"keyPathWii"`
	CertPathDS    string `xml:"certDerPathDS"`
	WiiCertPathDS string `xml:"wiiCertDerPathDS"`
	KeyPathDS     string `xml:"keyPathDS"`

	APISecret string `xml:"apiSecret"`

	AllowDefaultDolphinKeys     bool   `xml:"allowDefaultDolphinKeys"`
	AllowMultipleDeviceIDs      string `xml:"allowMultipleDeviceIDs"`
	AllowConnectWithoutDeviceID bool   `xml:"allowConnectWithoutDeviceID"`

	ServerName    string `xml:"serverName,omitempty"`
	MkwServerPath string `xml:"mkwServerPath,omitempty"`
	// Extra command line arguments for every mkw-server process, e.g. "--item-slot ItemSlot.bin".
	MkwServerArgs string `xml:"mkwServerArgs,omitempty"`
	// Address players outside this machine reach mkw-server rooms at (e.g. your public IP
	// when port forwarding). Players on this machine are always sent 127.0.0.1.
	MkwServerPublicAddress string `xml:"mkwServerPublicAddress,omitempty"`
	MkwServerClientVersion string `xml:"mkw-server-client-version,omitempty"`

	// Fill every new room with this many test bots (0-11), so one person can test a full
	// room. mkw-server plays the bots by mirroring the room's real player. Testing only.
	TestBots int `xml:"testBots,omitempty"`

	// Folder of mkw-server race recordings (its -record-dir, recorded with -record-packets).
	// The in-game Replays button plays back the newest one. Empty: no replays.
	ReplayDir string `xml:"replayDir,omitempty"`

	// Friend codes (comma separated) treated as linked to Lounge for mogis, for testing
	// while the Lounge friend code list is incomplete.
	MogiTestFriendCodes string `xml:"mogiTestFriendCodes,omitempty"`

	// For testing Lounge ranks: friend codes that count as a Lounge player, as
	// "FC=Lounge name" pairs, comma separated (e.g. "1234-5678-9012=Name").
	LoungeTestPlayers string `xml:"loungeTestPlayers,omitempty"`
}

var (
	config          Config
	configLoaded    bool
	configMu        sync.Mutex
	configModTime   time.Time
	configCheckedAt time.Time
)

var configCheckInterval = 2 * time.Second

func parseConfig(data []byte) (Config, error) {
	var config Config
	config.AllowDefaultDolphinKeys = true
	config.AllowMultipleDeviceIDs = "never"
	config.AllowConnectWithoutDeviceID = false
	config.ServerName = "WiiLink"

	if err := xml.Unmarshal(data, &config); err != nil {
		return Config{}, err
	}

	if config.GameSpyAddress == nil {
		config.GameSpyAddress = &config.DefaultAddress
	}

	if config.NASAddress == nil {
		config.NASAddress = &config.DefaultAddress
	}

	if config.NASAddressHTTPS == nil {
		if config.NASAddress != nil {
			config.NASAddressHTTPS = config.NASAddress
		} else {
			config.NASAddressHTTPS = &config.DefaultAddress
		}
	}

	if config.EnableHTTPSExploitWii == nil {
		enable := true
		config.EnableHTTPSExploitWii = &enable
	}

	if config.EnableHTTPSExploitDS == nil {
		enable := true
		config.EnableHTTPSExploitDS = &enable
	}

	if config.LogLevel == nil {
		level := 4
		config.LogLevel = &level
	}

	if config.LogOutput == "" {
		config.LogOutput = "StdOutAndFile"
	}

	if config.FrontendAddress == "" {
		config.FrontendAddress = "127.0.0.1:29998"
	}

	if config.BackendAddress == "" {
		config.BackendAddress = "127.0.0.1:29999"
	}

	if config.FrontendBackendAddress == "" {
		config.FrontendBackendAddress = config.BackendAddress
	}

	if config.BackendFrontendAddress == "" {
		config.BackendFrontendAddress = config.FrontendAddress
	}

	if config.AllowMultipleDeviceIDs == "true" || config.AllowMultipleDeviceIDs == "yes" {
		config.AllowMultipleDeviceIDs = "always"
	} else if config.AllowMultipleDeviceIDs != "SameIPAddress" {
		config.AllowMultipleDeviceIDs = "never"
	}

	return config, nil
}

// GetConfig returns the settings from config.xml. The file is checked for changes at most
// every configCheckInterval and reloaded when it changed, so edits apply without a restart
// (to new logins and new rooms; mkw-server gets its arguments when a room is created). A
// file that doesn't parse keeps the previous settings.
func GetConfig() Config {
	configMu.Lock()
	defer configMu.Unlock()
	now := time.Now()
	if configLoaded && now.Sub(configCheckedAt) < configCheckInterval {
		return config
	}
	configCheckedAt = now
	info, err := os.Stat("config.xml")
	if err != nil {
		if !configLoaded {
			panic(err)
		}
		return config
	}
	if configLoaded && info.ModTime().Equal(configModTime) {
		return config
	}
	data, err := os.ReadFile("config.xml")
	if err == nil {
		var parsed Config
		if parsed, err = parseConfig(data); err == nil {
			if configLoaded {
				fmt.Println("Reloaded config.xml")
			}
			config, configModTime, configLoaded = parsed, info.ModTime(), true
			return config
		}
	}
	if !configLoaded {
		panic(err)
	}
	fmt.Println("config.xml has an error, keeping the previous settings:", err)
	configModTime = info.ModTime() // Don't retry until it changes again.
	return config
}
