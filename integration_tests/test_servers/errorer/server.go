package main

import (
	"os"
	"time"
)

func main() {
	time.Sleep(50 * time.Millisecond)

	os.Exit(1)
}
