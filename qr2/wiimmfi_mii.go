package qr2

import (
	"encoding/base64"
	"encoding/xml"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"wwfc/logging"
)

// Lounge players' Miis, for the leaderboard: Wiimmfi keeps each player's Mii in the game's
// SAKE storage (FriendInfo table, by profile id = the low 32 bits of the friend code), and
// answers the game's own SearchForRecords requests (as MKW Table Bot does). Players whose
// friend code isn't known, or who have no Mii there, keep the default (the viewer's own).

const (
	wiimmfiSakeURL = "http://mariokartwii.sake.gs.wiimmfi.de/SakeStorageServer/StorageServer.asmx"
	sakeBatch      = 50
	rawMiiLen      = 0x4c // The FriendInfo record starts with the raw Mii.
)

var sakeClient = &http.Client{Timeout: 20 * time.Second}

const sakeRequest = `<?xml version="1.0" encoding="UTF-8"?>
<SOAP-ENV:Envelope xmlns:SOAP-ENV="http://schemas.xmlsoap.org/soap/envelope/" xmlns:ns1="http://gamespy.net/sake">
  <SOAP-ENV:Body>
    <ns1:SearchForRecords>
      <ns1:gameid>1687</ns1:gameid>
      <ns1:secretKey>9Rmy</ns1:secretKey>
      <ns1:loginTicket>23c715d620f986c22Pwwii</ns1:loginTicket>
      <ns1:tableid>FriendInfo</ns1:tableid>
      <ns1:filter>%s</ns1:filter>
      <ns1:sort>recordid</ns1:sort>
      <ns1:offset>0</ns1:offset>
      <ns1:max>%d</ns1:max>
      <ns1:surrounding>0</ns1:surrounding>
      <ns1:ownerids></ns1:ownerids>
      <ns1:cacheFlag>0</ns1:cacheFlag>
      <ns1:fields>
        <ns1:string>info</ns1:string>
        <ns1:string>ownerid</ns1:string>
      </ns1:fields>
    </ns1:SearchForRecords>
  </SOAP-ENV:Body>
</SOAP-ENV:Envelope>`

type sakeResponse struct {
	Records []struct {
		Values []struct {
			Binary string `xml:"binaryDataValue>value"`
			Int    string `xml:"intValue>value"`
		} `xml:"RecordValue"`
	} `xml:"Body>SearchForRecordsResponse>values>ArrayOfRecordValue"`
}

// fetchWiimmfiMiis gets the raw Miis of these profile ids. Batches that fail are skipped.
func fetchWiimmfiMiis(pids []uint32) map[uint32][]byte {
	miis := map[uint32][]byte{}
	for start := 0; start < len(pids); start += sakeBatch {
		batch := pids[start:min(start+sakeBatch, len(pids))]
		got, err := fetchSakeBatch(batch)
		if err != nil {
			logging.Error(moduleName, "Wiimmfi Mii lookup failed:", err.Error())
			continue
		}
		for pid, mii := range got {
			miis[pid] = mii
		}
	}
	return miis
}

func fetchSakeBatch(pids []uint32) (map[uint32][]byte, error) {
	filters := make([]string, len(pids))
	for i, pid := range pids {
		filters[i] = fmt.Sprintf("ownerid=%d", pid)
	}
	body := fmt.Sprintf(sakeRequest, strings.Join(filters, " || "), len(pids))
	req, err := http.NewRequest(http.MethodPost, wiimmfiSakeURL, strings.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "GameSpyHTTP/1.0")
	req.Header.Set("Content-Type", "text/xml")
	req.Header.Set("SOAPAction", "http://gamespy.net/sake/SearchForRecords")
	resp, err := sakeClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("SAKE: %s", resp.Status)
	}
	var parsed sakeResponse
	if err := xml.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return nil, err
	}
	return parseSakeMiis(parsed), nil
}

func parseSakeMiis(parsed sakeResponse) map[uint32][]byte {
	miis := map[uint32][]byte{}
	for _, rec := range parsed.Records {
		var info []byte
		var owner uint64
		for _, v := range rec.Values {
			if v.Binary != "" {
				info, _ = base64.StdEncoding.DecodeString(strings.TrimSpace(v.Binary))
			}
			if v.Int != "" {
				owner, _ = strconv.ParseUint(strings.TrimSpace(v.Int), 10, 32)
			}
		}
		if owner != 0 && len(info) >= rawMiiLen {
			miis[uint32(owner)] = info[:rawMiiLen]
		}
	}
	return miis
}
