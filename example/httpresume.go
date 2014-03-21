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

	m := upgradeable_http.NewManager()
	var wg sync.WaitGroup
	wg.Add(2)
	go serve(m, "one", ":8080", &wg)
	go serve(m, "two", ":8081", &wg)

	wg.Wait()

	log.Println("exiting...")
	os.Exit(0)
}

func serve(m upgradeable_http.Manager, ident, addr string, wg *sync.WaitGroup) {
	err := m.ListenAndServe(ident, addr, http.HandlerFunc(serverResponse))
	if err != nil {
		log.Fatal("Serve error: ", err)
	}
	wg.Done()
}

func serverResponse(w http.ResponseWriter, r *http.Request) {
	time.Sleep(500 * time.Millisecond)
	fmt.Fprintf(w, "Hello from %d, started at %v\n", syscall.Getpid(), startTime)
}
