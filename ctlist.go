package ctlogacquisition

import (
	"encoding/json"
	"io/ioutil"
	"net/http"
	"time"
)

// GOLANG tips: if your json
type stream struct {
	url     string
	min     int
	max     int
	current int
	count   int
}

type ctlist struct {
	Logs      []ctlistendpoint `json:"logs"`
	Operators []operator       `json:"operators"`
}
type ctlistendpoint struct {
	Description         string `json:"description"`
	Key                 string `json:"key"`
	Url                 string `json:"url"`
	Maximum_merge_delay int    `json:"maximum_merge_delay"`
	Operated_by         []int  `json:"operated_by"`
}

type operator struct {
	Name string `json:"name"`
	Id   int    `json:"id"`
}

func GetListCT() (*ctlist, error) {
	url := "https://www.gstatic.com/ct/log_list/all_logs_list.json"
	// //Debug only
	// url = "http://127.0.0.1:9999/listctlog.json"
	// Using http.Client so we can modify timeout value
	httpclient := http.Client{
		Timeout: time.Second * 2, // Maximum of 2 secs timeout
	}
	resp, err := httpclient.Get(url)
	if err != nil {
		paniciferr(err)
	}

	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)

	var list ctlist
	json.Unmarshal([]byte(body), &list)
	if err != nil {
		paniciferr(err)
	}

	return &list, nil
}
