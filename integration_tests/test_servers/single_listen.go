package main

import (
	"fmt"
	"flag"
	"log"
	"net/http"
	"syscall"
	"time"

	"github.com/alext/upgradeable_http"
)

var (
	startTime time.Time
	listenAddr *string = flag.String("listenAddr", ":8081", "The address to listen on")
)

func main() {
	startTime = time.Now()
	flag.Parse()

	upgradeable_http.StartupDelay = 100 * time.Millisecond
	upgradeable_http.CloseWaitTimeout = 500 * time.Millisecond
	m := upgradeable_http.NewManager()

	err := m.ListenAndServe("default", *listenAddr, http.HandlerFunc(serverResponse))
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
