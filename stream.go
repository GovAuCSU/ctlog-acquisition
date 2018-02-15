package ctlogacquisition

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"math/rand"
	"net/http"
	"strings"
	"time"
)

const PROTOCOL = "https://"
const INFOURI = "ct/v1/get-sth"
const DOWNLOADURI = "ct/v1/get-entries"

type endpoint struct {
	result              chan string
	Infourl             string
	Downloadurl         string
	Tree_size           int    `json:"tree_size"`
	Timestamp           int    `json:"timestamp"`
	Sha256_root_hash    string `json:"sha256_root_hash"`
	Tree_head_signature string `json:"tree_head_signature"`
}

func Newendpoint(path string) (*endpoint, error) {
	infourl := PROTOCOL + path + INFOURI
	downloadurl := PROTOCOL + path + DOWNLOADURI

	httpclient := http.Client{
		Timeout: time.Second * 10, // Timeout after 10 secs timeout
	}
	count := 0
	for {
		count = count + 1
		resp, err := httpclient.Get(infourl)
		if err != nil {
			if strings.Contains(err.Error(), "Client.Timeout") && (count < 5) {
				time.Sleep(time.Duration(rand.Int()%3000) * time.Millisecond)
				log.Printf("[WARN] Retrying %s at count %d\n", path, count)
				continue
			}
			log.Printf(">>>>>>>>>>>>>>")
			log.Printf(err.Error())
			return nil, err
		}
		defer resp.Body.Close()
		body, err := ioutil.ReadAll(resp.Body)
		var ep endpoint
		json.Unmarshal([]byte(body), &ep)
		if err != nil {
			return nil, err
		}
		ep.Infourl = infourl
		ep.Downloadurl = downloadurl

		return &ep, nil
	}
}

// func (ep *endpoint) StreamLog(message chan string, start, end, pagesize int) error {
// 	domainlist, err := ep.GetPageLog(start, end, pagesize)
// 	if err != nil {
// 		return err
// 	}
// 	for _, s := range domainlist {
// 		message <- s
// 	}
// 	return nil
// }

func (ep *endpoint) StreamLog(message chan string, start, end, pagesize int) error {
	size := end - start
	if size <= 0 {
		return fmt.Errorf("[ERROR] StreamLog : End should be larger than start")
	}
	// Using http.Client so we can modify timeout value
	httpclient := http.Client{
		Timeout: time.Second * 10, // Timeout after 30 secs timeout
	}
	t := end
	if start+pagesize < end {
		t = start + pagesize
	}
	count := 0
	for {
		count = count + 1
		resp, err := httpclient.Get(ep.Downloadurl + fmt.Sprintf("?start=%d&end=%d", start, t))
		if err != nil {
			if strings.Contains(err.Error(), "Client.Timeout") && (count < 5) {
				time.Sleep(time.Duration(rand.Int()%3000) * time.Millisecond)
				log.Printf("[WARN] Retrying %s at count %d\n", ep.Downloadurl, count)
				continue
			}
			log.Printf(">>>>>>>>>>>>>>")
			log.Printf(err.Error())
			return err
		}

		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("[DEBUG] Got wrong status code for %s: %d", ep.Downloadurl, resp.StatusCode)
		}

		var leaves struct {
			// blah *struct{} // nil if notset
			// blah struct{}  //empty
			Entries []struct {
				Leaf_input string `json:"leaf_input"`
			}
		}

		err = json.NewDecoder(resp.Body).Decode(&leaves)
		if err != nil {
			return err
		}

		resultlength := len(leaves.Entries)
		if resultlength == 0 {
			return fmt.Errorf("[DEBUG] Empty data for %s", ep.Downloadurl)
		}
		for _, leaf := range leaves.Entries {
			domainlist, err := getDomainFromLeaf(leaf.Leaf_input)
			if err != nil {
				return err
			}
			for _, s := range domainlist {
				message <- s
			}

		}
		// If there are still more records,
		if (resultlength < size) && (t < end) {
			log.Printf("[WARN] Getting more log from %s:%d --> %d with pagesize %d\n", ep.Downloadurl, start+resultlength, end, resultlength)
			ep.StreamLog(message, start+resultlength, end, resultlength)
		}
	}
	return nil

}
