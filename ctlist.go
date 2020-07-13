package ctlogacquisition

import (
	"encoding/json"
	"io/ioutil"
	"net/http"
	"time"
)

type ctlist struct {
	Logs      []ctlistendpoint `json:"logs"`
	Operators []operator       `json:"operators"`
}
type ctlistendpoint struct {
	Description         string `json:"description"`
	Key                 string `json:"key"`
	Url                 string `json:"url"`
	Disqualified        int    `json:"disqualified_at"`
	Maximum_merge_delay int    `json:"maximum_merge_delay"`
	Operated_by         []int  `json:"operated_by"`
}

type operator struct {
	Name string `json:"name"`
	Id   int    `json:"id"`
}

func GetListCT() (*ctlist, error) {
	url := "https://www.gstatic.com/ct/log_list/log_list.json"
	// Using http.Client so we can modify timeout value
	httpclient := http.Client{
		Timeout: time.Second * 2, // Maximum of 2 secs timeout
	}
	resp, err := httpclient.Get(url)
	if err != nil {
		return nil, err
	}

	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var list ctlist
	err = json.Unmarshal([]byte(body), &list)
	if err != nil {
		return nil, err
	}

	return &list, nil
}
