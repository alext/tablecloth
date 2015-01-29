package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"syscall"
	"time"

	"github.com/alext/tablecloth"
)

const greeting = "Hello"

var (
	startTime       time.Time
	listenAddr      = flag.String("listenAddr", ":8081", "The address to listen on")
	closeTimeoutStr = flag.String("closeTimeout", "500ms", "How long to wait for connections to gracefully close")
	workingDir      = flag.String("workingDir", "", "The directory to change to before re-execing")
)

func main() {
	startTime = time.Now()
	flag.Parse()

	tablecloth.StartupDelay = 100 * time.Millisecond

	closeTimeout, err := time.ParseDuration(*closeTimeoutStr)
	if err != nil {
		log.Fatal("Invalid closeTimeout: ", err)
	}
	tablecloth.CloseWaitTimeout = closeTimeout

	if *workingDir != "" {
		tablecloth.WorkingDir = *workingDir
	}

	err = tablecloth.ListenAndServe(*listenAddr, http.HandlerFunc(serverResponse))
	if err != nil {
		log.Fatal("Serve error: ", err)
	}
}

func serverResponse(w http.ResponseWriter, r *http.Request) {
	time.Sleep(250 * time.Millisecond)

	// Force closing of the connection to prevent keepalive
	w.Header().Set("Connection", "close")
	fmt.Fprintf(w, "%s (pid=%d)\nStarted at %v\n", greeting, syscall.Getpid(), startTime)
}
