package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"sync"
	"time"

	ctl "github.com/GovAuCSU/ctlog-acquisition"
	lru "github.com/hashicorp/golang-lru"
	"github.com/hashicorp/logutils"
)

const listenPort = ":3000"
const PAGESIZE = 300
const CONFIGFILE = "config.json"
const nameCacheSize = 8192

var disableWebServer *bool
var startCurrent *bool
var onePass *bool

func usage() {
	fmt.Println("Usage: " + os.Args[0] + " [options]")
	fmt.Println("")
	fmt.Println("Connects to public CT logs, downloads records, and extracts potential hostnames.")
	fmt.Println("")
	fmt.Println("Results are accessible via the file system or a built-in web server on port " + listenPort)
	fmt.Println("")
	fmt.Println("Options:")
	flag.PrintDefaults()
}

// WriteChanToWriter will read strings from channel c and write to Writer w.
func WriteChanToWriter(ctx context.Context, w io.Writer, c chan string) {
	nameCache, _ := lru.New(nameCacheSize)
	for {
		select {
		case <-ctx.Done():
			return

		case s, ok := <-c:
			// If channel closed, return it
			if !ok {
				return
			}
			if !nameCache.Contains(s) {
				fmt.Fprintf(w, "%s\n", s)
				nameCache.Add(s, nil)
			}
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
	if _, err := os.Stat(filename); os.IsNotExist(err) {
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

// Scheduler query CT logs every 'delay' unless intializing or making a single pass
func Scheduler(ctx context.Context, confcomm ConfigChannel, filepath string, delay time.Duration) {

	log.Println("[INFO] Starting Scheduler")
	for {
		GetLogToFile(ctx, confcomm, filepath)
		if *onePass {
			return
		}

		// At this point the config values should be set correctly. Setting
		// startCurrent to false will stop this from happening every loop.
		*startCurrent = false
		select {
		case <-time.After(delay):
		}
	}
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

	// Starting one thread per CT Log api endpoint
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

	if *startCurrent {
		confcomm.update <- ep
		epconf.Tree_size = ep.Tree_size
	}

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
	// defering cancellation of all concurrent processes
	defer cancel()
	os.MkdirAll(localpath, os.ModePerm)
	confcomm := ConfigChannel{
		request: make(chan *ConfigComm),
		update:  make(chan *ctl.Endpoint),
	}

	go ManageConfiguration(ctx, confcomm)

	if !*disableWebServer {
		fs := http.FileServer(http.Dir(localpath))
		http.Handle("/", fs)

		log.Printf("[INFO] Listening on port %v", listenPort)
		go http.ListenAndServe(listenPort, nil)
	} else {
		log.Println("[INFO] Webserver disabled.")
	}

	if ctl.DisableAPICertValidation {
		log.Println("[INFO] CT Log API certificate validation disabled.")
	}

	Scheduler(ctx, confcomm, localpath, 300*time.Second)

	return nil
}

// Neat trick to consistently handling error
func main() {

	flag.Usage = func() { usage() }
	disableWebServer = flag.Bool("disable-webserver", false, "Disable built-in webserver.")
	var DisableAPICertValidation = flag.Bool("disable-cert-validation", false, "Disable validation of CT log API endpoint x.509 certificates (not retrieved certificates).")
	startCurrent = flag.Bool("start-current", false, "Set current CT log record numbers as starting point in the config. This enables only processing newly added certificates.")
	onePass = flag.Bool("one-pass", false, "Process CT logs until current and exit.")
	flag.Parse()

	ctl.DisableAPICertValidation = *DisableAPICertValidation

	err := realmain()
	if err != nil {
		log.Println(err)
	}
}
