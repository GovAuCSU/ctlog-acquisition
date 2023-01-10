package ctlogacquisition

import (
	"encoding/json"
	"io/ioutil"
	"net/http"
	"time"
)

type ctlist struct {
	Operators []operator       `json:"operators"`
}
type ctlistendpoint struct {
	Description         string `json:"description"`
	Key                 string `json:"key"`
	Url                 string `json:"url"`
}

type operator struct {
	Name string `json:"name"`
	Id   int    `json:"id"`
	Logs []ctlistendpoint `json:"logs"`
}

func GetListCT() (*ctlist, error) {
	url := "https://www.gstatic.com/ct/log_list/v3/log_list.json"
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
