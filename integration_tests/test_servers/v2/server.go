package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"syscall"
	"time"

	"github.com/alext/upgradeable_http"
)

var (
	startTime  time.Time
	listenAddr *string = flag.String("listenAddr", ":8081", "The address to listen on")
	workingDir *string = flag.String("workingDir", "", "The directory to change to before re-execing")
)

func main() {
	startTime = time.Now()
	flag.Parse()

	upgradeable_http.StartupDelay = 100 * time.Millisecond
	upgradeable_http.CloseWaitTimeout = 500 * time.Millisecond
	if *workingDir != "" {
		upgradeable_http.WorkingDir = *workingDir
	}

	err := upgradeable_http.ListenAndServe(*listenAddr, http.HandlerFunc(serverResponse))
	if err != nil {
		log.Fatal("Serve error: ", err)
	}
}

func serverResponse(w http.ResponseWriter, r *http.Request) {
	time.Sleep(250 * time.Millisecond)

	// Force closing of the connection to prevent keepalive
	w.Header().Set("Connection", "close")
	fmt.Fprintf(w, "Hello from v2 pid: %d\nStarted at %v\n", syscall.Getpid(), startTime)
}
