package main

import (
	"fmt"
	"flag"
	"log"
	"net/http"
	"sync"
	"syscall"
	"time"

	"github.com/alext/upgradeable_http"
)

var (
	startTime time.Time
	listenAddr1 *string = flag.String("listenAddr1", ":8081", "The address to listen on")
	listenAddr2 *string = flag.String("listenAddr2", ":8082", "The address to listen on")
)

func main() {
	startTime = time.Now()
	flag.Parse()

	upgradeable_http.StartupDelay = 100 * time.Millisecond
	upgradeable_http.CloseWaitTimeout = 500 * time.Millisecond
	m := upgradeable_http.NewManager()

	wg := &sync.WaitGroup{}
	wg.Add(2)
	go serve(m, "one", *listenAddr1, wg)
	go serve(m, "two", *listenAddr2, wg)

	wg.Wait()
}

func serve(m upgradeable_http.Manager, ident, listenAddr string, wg *sync.WaitGroup) {
	defer wg.Done()
	err := m.ListenAndServe(ident, listenAddr, http.HandlerFunc(serverResponse))
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
