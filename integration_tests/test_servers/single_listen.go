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
	m := upgradeable_http.NewManager()

	err := m.ListenAndServe("default", *listenAddr, http.HandlerFunc(serverResponse))
	if err != nil {
		log.Fatal("Serve error: ", err)
	}
}

func serverResponse(w http.ResponseWriter, r *http.Request) {
	fmt.Fprintf(w, "Hello from %d\nStarted at %v\n", syscall.Getpid(), startTime)
}
