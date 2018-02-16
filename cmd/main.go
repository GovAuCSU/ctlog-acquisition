package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"sync"
	"time"

	ctl "github.com/GovAuCSU/ctlog-acquisition"
	"github.com/hashicorp/logutils"
)

const PAGESIZE = 300
const CONFIGFILE = "config.json"

// WriteChanToWriter will read strings from channel c and write to Writer w.
func WriteChanToWriter(ctx context.Context, w io.Writer, c chan string) {
	for {
		select {
		case <-ctx.Done():
			return

		case s, ok := <-c:
			// If channel closed, return it
			if !ok {
				return
			}
			fmt.Fprintf(w, "%s\n", s)
		}
	}
}

// Using this struct to send comm between goroutines to get/update endpoint configurations
// The go routine send a request along with a reply channel so our ManageConfig routine know
// how to reply
type ConfigComm struct {
	query string
	reply chan ctl.Endpoint
}

type Configuration struct {
	Endpoints map[string]ctl.Endpoint
}

type ConfigChannel struct {
	request chan *ConfigComm
	update  chan *ctl.Endpoint
}

func loadConfig(filename string) *Configuration {
	// If no configuration is found, return empty config
	if _, err := os.Stat("filename"); os.IsNotExist(err) {
		return &Configuration{
			Endpoints: make(map[string]ctl.Endpoint),
		}
	}
	bytes, err := ioutil.ReadFile(filename)
	if err != nil {
		log.Println("Exiting app due to Error in reading configuration file.")
		panic(err)
	}

	var c Configuration
	err = json.Unmarshal(bytes, &c)
	if err != nil {
		log.Println("Exiting app due to corrupted configuration file.")
		panic(err)
	}

	return &c
}

func saveConfig(c Configuration, filename string) error {
	bytes, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}

	return ioutil.WriteFile(filename, bytes, 0644)
}

func ManageConfiguration(ctx context.Context, comm ConfigChannel) {
	conf := loadConfig(CONFIGFILE)
	// Load config
	for {
		select {
		case req := <-comm.request:
			req.reply <- conf.Endpoints[req.query]
		case u := <-comm.update:
			conf.Endpoints[u.Url] = *u
			saveConfig(*conf, CONFIGFILE)
		}
	}
}

func Scheduler(ctx context.Context, confcomm ConfigChannel, filepath string, delay time.Duration) chan bool {
	stop := make(chan bool)
	go func() {
		for {
			GetLogToFile(ctx, confcomm, filepath)
			stop := make(chan bool)
			select {
			case <-time.After(delay):
			case <-stop:
				return
			}
		}
	}()
	return stop
}

func GetLogToFile(ctx context.Context, confcomm ConfigChannel, filepath string) {
	start := 1000
	end := 2000
	err := RealGetLogToFile(ctx, confcomm, start, end, filepath)
	if err != nil {
		log.Println(err)
		return
	}
}

func RealGetLogToFile(ctx context.Context, confcomm ConfigChannel, start, end int, folderpath string) error {
	// -----------
	t := time.Now()
	log.Println("[INFO] Preparing our output file")
	localfile := fmt.Sprintf("%s/ct_log_%s.txt", folderpath, t.Format("2006_02_01"))
	f, err := os.OpenFile(localfile, os.O_WRONLY|os.O_APPEND|os.O_CREATE|os.O_TRUNC, 0644)
	if err != nil {
		log.Printf("[ERROR] Fail to open file %s\n", localfile)
	}
	msg := make(chan string, 1000)
	go WriteChanToWriter(ctx, f, msg)

	// Getting list of ct endpoints
	log.Println("[INFO] Acquiring a list of CT log servers")
	ctlist, err := ctl.GetListCT()
	if err != nil {
		return err
	}
	_ = ctlist
	var wg sync.WaitGroup

	for _, l := range ctlist.Logs {
		wg.Add(1)
		go GetLog(msg, confcomm, l.Url, start, end, &wg)
	}

	wg.Wait()
	return nil
}

func GetLog(message chan string, confcomm ConfigChannel, url string, start, end int, wg *sync.WaitGroup) {
	log.Printf("[INFO] Starting request for %s\n", url)
	defer wg.Done()
	ep, err := ctl.Newendpoint(url)
	if err != nil {
		log.Printf("[DEBUG] %s\n%v", url, err)
		return
	}

	comm := &ConfigComm{
		query: ep.Url,
		reply: make(chan ctl.Endpoint),
	}
	confcomm.request <- comm
	epconf := <-comm.reply

	if epconf.Tree_size < ep.Tree_size {
		sum, err := ep.StreamLog(message, epconf.Tree_size, ep.Tree_size, PAGESIZE)
		if err != nil {
			log.Println(err)
		}
		ep.Tree_size = epconf.Tree_size + sum

	}
	confcomm.update <- ep
	log.Printf("[INFO] Closing goroutine for %s\n", url)

}

func realmain() error {

	filter := &logutils.LevelFilter{
		Levels:   []logutils.LogLevel{"DEBUG", "WARN", "ERROR", "INFO"},
		MinLevel: logutils.LogLevel("DEBUG"),
		Writer:   os.Stderr,
	}
	log.SetOutput(filter)
	// variables
	localpath := "./static"
	ctx, cancel := context.WithCancel(context.Background())
	// defering canclation of all concurence processes
	defer cancel()
	os.MkdirAll(localpath, os.ModePerm)
	confcomm := ConfigChannel{
		request: make(chan *ConfigComm),
		update:  make(chan *ctl.Endpoint),
	}

	go ManageConfiguration(ctx, confcomm)

	stopLog := Scheduler(ctx, confcomm, localpath, 300*time.Second)

	fs := http.FileServer(http.Dir(localpath))
	http.Handle("/", fs)

	log.Println("[INFO] Listening...")
	http.ListenAndServe(":3000", nil)
	stopLog <- true
	return nil
}

// Neat trick to consistently handling error
func main() {
	err := realmain()
	if err != nil {
		log.Println(err)
	}
}
