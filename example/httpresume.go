package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"sync"
	"syscall"
	"time"

	"github.com/alext/upgradeable_http"
)

var (
	startTime = time.Now()
)

func init() {
	log.SetPrefix(fmt.Sprintf("[%d] ", syscall.Getpid()))
}

func getenvDefault(key string, defaultVal string) string {
	val := os.Getenv(key)
	if val == "" {
		val = defaultVal
	}

	return val
}

func main() {
	log.Println("main started with args:", os.Args)

	var wg sync.WaitGroup
	wg.Add(2)
	go serve(":8080", "one", &wg)
	go serve(":8081", "two", &wg)

	wg.Wait()

	log.Println("exiting...")
	os.Exit(0)
}

func serve(addr, ident string, wg *sync.WaitGroup) {
	err := upgradeable_http.ListenAndServe(addr, http.HandlerFunc(serverResponse), ident)
	if err != nil {
		log.Fatal("Serve error: ", err)
	}
	wg.Done()
}

func serverResponse(w http.ResponseWriter, r *http.Request) {
	time.Sleep(500 * time.Millisecond)
	fmt.Fprintf(w, "Hello from %d, started at %v\n", syscall.Getpid(), startTime)
}
