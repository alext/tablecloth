package integration_test

import (
	"fmt"
	"io/ioutil"
	"net/http"
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
	RunSpecs(t, "Upgradeable HTTP")
}

var _ = Describe("Upgradeable HTTP listener", func() {

	Context("Listening on a single port", func() {
		var (
			client	  *http.Client
			serverCmd *exec.Cmd
		)

		BeforeEach(func() {
			client = httpClient()
			var err error
			serverCmd, err = startServer("single_listen", 8081)
			Expect(err).To(BeNil())
		})

		AfterEach(func() {
			stopServer(serverCmd)
		})

		It("Should listen on the given address", func() {
			resp, err := client.Get("http://127.0.0.1:8081/")
			Expect(err).To(BeNil())

			Expect(resp.StatusCode).To(Equal(200))
		})

		It("Should restart when given a HUP signal", func() {
			resp, err := client.Get("http://127.0.0.1:8081/")
			Expect(err).To(BeNil())
			firstBody, _ := ioutil.ReadAll(resp.Body)
			resp.Body.Close()

			Expect(resp.StatusCode).To(Equal(200))

			reloadServer(serverCmd)

			resp, err = client.Get("http://127.0.0.1:8081/")
			Expect(err).To(BeNil())
			newBody, _ := ioutil.ReadAll(resp.Body)
			resp.Body.Close()

			Expect(resp.StatusCode).To(Equal(200))

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

func httpClient() *http.Client {
	return &http.Client{
		Transport: &http.Transport{DisableKeepAlives: true},
	}
}

func startServer(server string, port int) (cmd *exec.Cmd, err error) {
	cmd = exec.Command(fmt.Sprintf("./test_servers/%s", server), fmt.Sprintf("-listenAddr=127.0.0.1:%d", port))
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
