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

func buildTestServers() (err error) {
	cmd := exec.Command("make")
	cwd, _ := os.Getwd()
	cmd.Dir = cwd + "/test_servers"
	cmd.Dir = "./test_servers"
	err = cmd.Run()
	return
}

var _ = Describe("Upgradeable HTTP listener", func() {

	Context("Listening on a single port", func() {
		var (
			serverCmd *exec.Cmd
		)

		BeforeEach(func() {
			var err error
			serverCmd, err = startServer("single_listen", 8081)
			Expect(err).To(BeNil())
		})

		AfterEach(func() {
			stopServer(serverCmd)
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
			resultCh := startVegetaAttack([]string{"GET http://127.0.0.1:8081"}, 100, 5 * time.Second)

			time.Sleep(500 * time.Millisecond)
			reloadServer(serverCmd)

			metrics := <- resultCh
			Expect(metrics.StatusCodes["200"]).To(Equal(int(metrics.Requests)))
		})
	})
})

func startServer(server string, port int) (cmd *exec.Cmd, err error) {
	cmd = exec.Command(fmt.Sprintf("./test_servers/%s", server), fmt.Sprintf("-listenAddr=127.0.0.1:%d", port))
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	err = cmd.Start()
	time.Sleep(1 * time.Second)

	return
}

func reloadServer(cmd *exec.Cmd) {
	cmd.Process.Signal(syscall.SIGHUP)
	time.Sleep(3 * time.Second)
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
