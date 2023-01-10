package ctlogacquisition

import (
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"time"
)

const INFOURI = "ct/v1/get-sth"
const DOWNLOADURI = "ct/v1/get-entries"

// Determines if certificates should be validated when downloading from Log API endpoints
var DisableAPICertValidation bool = true

type Endpoint struct {
	Url                 string `json:url`
	Infourl             string `json:"info_url"`
	Downloadurl         string `json:"download_url"`
	Tree_size           int    `json:"tree_size"`
	Timestamp           int    `json:"timestamp"`
	Sha256_root_hash    string `json:"sha256_root_hash"`
	Tree_head_signature string `json:"tree_head_signature"`
}

var tr = &http.Transport{
	TLSClientConfig: &tls.Config{InsecureSkipVerify: DisableAPICertValidation},
}

var httpclient = http.Client{
	Timeout:   time.Second * 10, // Timeout after 10 secs timeout
	Transport: tr,
}

func Newendpoint(path string) (*Endpoint, error) {
	infourl := path + INFOURI
	downloadurl := path + DOWNLOADURI

	resp, err := httpclient.Get(infourl)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	var ep Endpoint
	ep.Url = path
	err = json.Unmarshal([]byte(body), &ep)
	if err != nil {
		return nil, err
	}
	ep.Infourl = infourl
	ep.Downloadurl = downloadurl

	return &ep, nil

}

// StreamLog connects to the Downloadurl of the specified endpoint, requests
// the records between start and end (inclusively), extracts the potential
// hostnames, sends the hostnames down the 'message' channel, and returns
// the number of log entry records retrieved.
func (ep *Endpoint) StreamLog(message chan string, start, end int) (int, error) {
	size := end - start
	if size <= 0 {
		return 0, fmt.Errorf("[ERROR] StreamLog : End should be larger than start")
	}

	log.Printf("[INFO] Getting log from %s:%d --> %d\n", ep.Downloadurl, start, end)
	resp, err := httpclient.Get(ep.Downloadurl + fmt.Sprintf("?start=%d&end=%d", start, end))

	if err != nil {
		return 0, err
	}

	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := ioutil.ReadAll(resp.Body)
		return 0, fmt.Errorf("[DEBUG] Got wrong status code for %s: Code: %d Body: %q", ep.Downloadurl, resp.StatusCode, bodyBytes)
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
		return 0, err
	}

	responselength := len(leaves.Entries)
	if responselength == 0 {
		return 0, fmt.Errorf("[DEBUG] Empty data for %s", ep.Downloadurl)
	}
	for _, leaf := range leaves.Entries {
		domainlist, _ := getDomainFromLeaf(leaf.Leaf_input)
		if err != nil {
			// Ignore error so we can continue with the next leaf
			continue
			//return 0, err
		}
		for _, s := range domainlist {
			message <- s
		}

	}
	return responselength, nil
}
