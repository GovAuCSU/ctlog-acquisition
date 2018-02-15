package main

import (
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"sync"
	"time"

	ctl "github.com/GovAuCSU/ctlog-acquisition"
	"github.com/hashicorp/logutils"
)

const PAGESIZE = 300

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

func Scheduler(ctx context.Context, filepath string, delay time.Duration) chan bool {
	stop := make(chan bool)
	go func() {
		for {
			GetLogToFile(ctx, filepath)
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

func GetLogToFile(ctx context.Context, filepath string) {
	start := 1000
	end := 2000
	err := RealGetLogToFile(ctx, start, end, filepath)
	if err != nil {
		log.Println(err)
		return
	}
}

func RealGetLogToFile(ctx context.Context, start, end int, folderpath string) error {
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
		go GetLog(msg, l.Url, start, end, &wg)
	}
	//Getting from a single server
	// wg.Add(1)
	// go GetLog(msg, "ct.googleapis.com/daedalus/", start, end, &wg)

	wg.Wait()
	return nil
}

func GetLog(message chan string, url string, start, end int, wg *sync.WaitGroup) {
	log.Printf("[INFO] Starting request for %s\n", url)
	defer wg.Done()
	ep, err := ctl.Newendpoint(url)
	if err != nil {
		log.Printf("[DEBUG] %s\n%v", url, err)
		return
	}
	err = ep.StreamLog(message, start, end, PAGESIZE)
	if err != nil {
		log.Println(err)
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
	// defering canclation of all concurence processes
	defer cancel()
	os.MkdirAll(localpath, os.ModePerm)
	stopLog := Scheduler(ctx, localpath, 300*time.Second)

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
