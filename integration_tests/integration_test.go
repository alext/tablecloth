package integration_test

import (
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"os/exec"
	"syscall"
	"testing"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	vegeta "github.com/tsenart/vegeta/lib"
)

func TestUpgradeableHTTP(t *testing.T) {
	RegisterFailHandler(Fail)

	err := buildTestServers()
	if err != nil {
		t.Fatalf("Failed to build test servers: %v", err)
	}
	RunSpecs(t, "Upgradeable HTTP")
}

func buildTestServers() error {
	cmd := exec.Command("make", "-B")
	cwd, _ := os.Getwd()
	cmd.Dir = cwd + "/test_servers"
	cmd.Dir = "./test_servers"
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%v\n%s", err, output)
	}
	return nil
}

var _ = Describe("Upgradeable HTTP listener", func() {
	var (
		serverCmd *exec.Cmd
	)

	AfterEach(func() {
		stopServer(serverCmd)
	})

	Context("Listening on a single port", func() {

		BeforeEach(func() {
			serverCmd = startServer("single_listen", "-listenAddr=127.0.0.1:8081")
		})

		It("Should listen on the given address", func() {
			resp, err := http.Get("http://127.0.0.1:8081/")
			Expect(err).To(BeNil())

			Expect(resp.StatusCode).To(Equal(200))
		})

		It("Should restart when given a HUP signal", func() {
			resp, err := http.Get("http://127.0.0.1:8081/")
			Expect(err).To(BeNil())
			firstBody, _ := ioutil.ReadAll(resp.Body)
			resp.Body.Close()

			Expect(resp.StatusCode).To(Equal(200))

			reloadServer(serverCmd)

			resp, err = http.Get("http://127.0.0.1:8081/")
			Expect(err).To(BeNil())
			newBody, _ := ioutil.ReadAll(resp.Body)
			resp.Body.Close()

			Expect(resp.StatusCode).To(Equal(200))

			// The response body includes the start time of the server
			Expect(string(newBody)).NotTo(Equal(string(firstBody)))
		})

		It("Should not drop any requests while reloading", func() {
			resultCh := startVegetaAttack([]string{"GET http://127.0.0.1:8081"}, 40, 3 * time.Second)

			time.Sleep(100 * time.Millisecond)
			reloadServer(serverCmd)

			metrics := <- resultCh
			Expect(metrics.StatusCodes["200"]).To(Equal(int(metrics.Requests)))
		})
	})

	Context("Listening on multiple ports", func() {

		BeforeEach(func() {
			serverCmd = startServer("double_listen", "-listenAddr1=127.0.0.1:8081", "-listenAddr2=127.0.0.1:8082")
		})

		It("Should listen on the given addresses", func() {
			resp, err := http.Get("http://127.0.0.1:8081/")
			Expect(err).To(BeNil())

			Expect(resp.StatusCode).To(Equal(200))

			resp, err = http.Get("http://127.0.0.1:8082/")
			Expect(err).To(BeNil())

			Expect(resp.StatusCode).To(Equal(200))
		})

		It("Should restart when given a HUP signal", func() {
			resp, err := http.Get("http://127.0.0.1:8081/")
			Expect(err).To(BeNil())
			firstBody1, _ := ioutil.ReadAll(resp.Body)
			resp.Body.Close()

			Expect(resp.StatusCode).To(Equal(200))

			resp, err = http.Get("http://127.0.0.1:8082/")
			Expect(err).To(BeNil())
			firstBody2, _ := ioutil.ReadAll(resp.Body)
			resp.Body.Close()

			Expect(resp.StatusCode).To(Equal(200))

			reloadServer(serverCmd)

			resp, err = http.Get("http://127.0.0.1:8081/")
			Expect(err).To(BeNil())
			newBody, _ := ioutil.ReadAll(resp.Body)
			resp.Body.Close()

			Expect(resp.StatusCode).To(Equal(200))

			// The response body includes the start time of the server
			Expect(string(newBody)).NotTo(Equal(string(firstBody1)))

			resp, err = http.Get("http://127.0.0.1:8082/")
			Expect(err).To(BeNil())
			newBody, _ = ioutil.ReadAll(resp.Body)
			resp.Body.Close()

			Expect(resp.StatusCode).To(Equal(200))

			// The response body includes the start time of the server
			Expect(string(newBody)).NotTo(Equal(string(firstBody2)))
		})

		It("Should not drop any requests while reloading", func() {
			resultCh := startVegetaAttack([]string{"GET http://127.0.0.1:8081", "GET http://127.0.0.1:8082"}, 40, 3 * time.Second)

			time.Sleep(100 * time.Millisecond)
			reloadServer(serverCmd)

			metrics := <- resultCh
			Expect(metrics.StatusCodes["200"]).To(Equal(int(metrics.Requests)))
		})

	})
})

func startServer(server string, args ...string) (cmd *exec.Cmd) {
	cmd = exec.Command(fmt.Sprintf("./test_servers/%s", server), args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	err := cmd.Start()
	Expect(err).To(BeNil())
	time.Sleep(50 * time.Millisecond)

	return
}

func reloadServer(cmd *exec.Cmd) {
	cmd.Process.Signal(syscall.SIGHUP)
	// Wait until the reload has completed
	// 2 * StartupDelay(1s) - once in temp child, once in new parent
	// plus a little bit to allow for other delays (eg waiting for connection close)
	time.Sleep(1500 * time.Millisecond)
}

func stopServer(cmd *exec.Cmd) {
	if cmd != nil && cmd.Process != nil {
		cmd.Process.Signal(syscall.SIGINT)
		cmd.Process.Wait()
	}
}

func startVegetaAttack(targetStrings []string, rate uint64, duration time.Duration) chan *vegeta.Metrics {
	targets, err := vegeta.NewTargets(targetStrings, []byte{}, http.Header{})
	if err != nil {
		panic(err)
	}
	metricsChan := make(chan *vegeta.Metrics, 1)
	go vegetaAttack(targets, rate, duration, metricsChan)
	return metricsChan
}

func vegetaAttack(targets vegeta.Targets, rate uint64, duration time.Duration, metricsChan chan *vegeta.Metrics) {
	results := vegeta.Attack(targets, rate, duration)
	metrics := vegeta.NewMetrics(results)
	metricsChan <- metrics
}
