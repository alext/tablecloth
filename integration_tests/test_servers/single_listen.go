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
	startTime       time.Time
	listenAddr      *string = flag.String("listenAddr", ":8081", "The address to listen on")
	closeTimeoutStr *string = flag.String("closeTimeout", "500ms", "How long to wait for connections to gracefully close")
)

func main() {
	startTime = time.Now()
	flag.Parse()

	upgradeable_http.StartupDelay = 100 * time.Millisecond

	closeTimeout, err := time.ParseDuration(*closeTimeoutStr)
	if err != nil {
		log.Fatal("Invalid closeTimeout: ", err)
	}
	upgradeable_http.CloseWaitTimeout = closeTimeout

	m := upgradeable_http.NewManager()

	err = m.ListenAndServe("default", *listenAddr, http.HandlerFunc(serverResponse))
	if err != nil {
		log.Fatal("Serve error: ", err)
	}
}

func serverResponse(w http.ResponseWriter, r *http.Request) {
	time.Sleep(250 * time.Millisecond)

	// Force closing of the connection to prevent keepalive
	w.Header().Set("Connection", "close")
	fmt.Fprintf(w, "Hello from %d\nStarted at %v\n", syscall.Getpid(), startTime)
}
