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
	"path/filepath"
	"sync"
	"syscall"
	"time"

	ctl "github.com/GovAuCSU/ctlog-acquisition"
	lru "github.com/hashicorp/golang-lru"
	"github.com/hashicorp/logutils"
)

const listenPort = ":3000"
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
		log.Println("[ERROR] Exiting app due to Error in reading configuration file.")
		panic(err)
	}

	var c Configuration
	err = json.Unmarshal(bytes, &c)
	if err != nil {
		log.Println("[ERROR] Exiting app due to corrupted configuration file.")
		panic(err)
	}

	return &c
}

func saveConfig(c Configuration, filename string) error {
	bytes, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		log.Printf("[ERROR] Saving configuration file. Error: %s", err)
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
			err := saveConfig(*conf, CONFIGFILE)
			if err != nil {
				log.Println(err)
			}
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
		 func() { time.After(delay) }()
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
	t := time.Now().UTC()
	filename := fmt.Sprintf("ct_log_%s.txt", t.Format("2006.01.02_03.04.05"))
	localfile := filepath.Join(folderpath, filename)

	log.Printf("[INFO] Preparing our output file: %s", localfile)
	f, err := os.OpenFile(localfile, os.O_WRONLY|os.O_APPEND|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return fmt.Errorf("[ERROR] Fail to open file %s. Error: %s", localfile, err)
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
	f.Close()
	return nil
}

func GetLog(message chan string, confcomm ConfigChannel, url string, start, end int, wg *sync.WaitGroup) {
	log.Printf("[INFO] Starting request for %s\n", url)
	defer wg.Done()
	ep, err := ctl.Newendpoint(url)
	if err != nil {
		log.Printf("[ERROR] %s\n%v", url, err)
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

	// Tree_size is a count of available records but the request is indexed
	// from 0 so if we request the end as ep.Tree_size most logs will return
	// an error instead of just returning the last record.
	for epconf.Tree_size < ep.Tree_size-1 {
		sum, err := ep.StreamLog(message, epconf.Tree_size, ep.Tree_size-1)
		if err != nil {
			log.Println(err)
		}
		epconf.Tree_size += sum
		confcomm.update <- &epconf
	}

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
	err := os.MkdirAll(localpath, os.ModePerm)
	if err != nil {
		log.Println(err)
	}
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

// increaseOpenFilesLimit ensures that the code has sufficient file descriptors
// to operate. In current versions of go (1.11 and earlier) DNS requests use
// one FD per request and, when combined with the HTTP requests, can cause
// name resolution errors even for servers another goroutine is already connected
// to.
func increaseOpenFilesLimit() {

	var minOpenFileLimit = 2048
	var rLimit syscall.Rlimit

	err := syscall.Getrlimit(syscall.RLIMIT_NOFILE, &rLimit)
	if err != nil {
		log.Println("[ERROR] Unable to determine max open file descriptors: ", err)
		return
	}

	var origLimit = rLimit
	var updated = false

	if rLimit.Cur < uint64(minOpenFileLimit) {
		rLimit.Cur = uint64(minOpenFileLimit)
		updated = true
	}

	if rLimit.Max < uint64(minOpenFileLimit) {
		rLimit.Max = uint64(minOpenFileLimit)
		updated = true
	}

	if updated {
		err = syscall.Setrlimit(syscall.RLIMIT_NOFILE, &rLimit)
		if err != nil {
			log.Println("[ERROR] Error while changing max open file descriptors: ", err)
			return
		}
		log.Printf("[INFO] Changed max open file descriptors from %d (%d) to %d (%d)", origLimit.Cur, origLimit.Max, rLimit.Cur, rLimit.Max)

	}
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

	increaseOpenFilesLimit()

	err := realmain()
	if err != nil {
		log.Println(err)
	}
}
