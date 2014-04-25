package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"sync"
	"syscall"
	"time"

	"github.com/alext/upgradeable_http"
)

var (
	startTime   time.Time
	listenAddr1 *string = flag.String("listenAddr1", ":8081", "The address to listen on")
	listenAddr2 *string = flag.String("listenAddr2", ":8082", "The address to listen on")
)

func main() {
	startTime = time.Now()
	flag.Parse()

	upgradeable_http.StartupDelay = 100 * time.Millisecond
	upgradeable_http.CloseWaitTimeout = 500 * time.Millisecond

	wg := &sync.WaitGroup{}
	wg.Add(2)
	go serve(*listenAddr1, "one", wg)
	go serve(*listenAddr2, "two", wg)

	wg.Wait()
}

func serve(listenAddr, ident string, wg *sync.WaitGroup) {
	defer wg.Done()
	err := upgradeable_http.ListenAndServe(listenAddr, http.HandlerFunc(serverResponse), ident)
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
