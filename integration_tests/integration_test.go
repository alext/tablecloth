package integration_test

import (
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"testing"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	vegeta "gopkg.in/tsenart/vegeta.v2/lib"
)

func TestTablecloth(t *testing.T) {
	RegisterFailHandler(Fail)

	err := buildTestServers()
	if err != nil {
		t.Fatalf("Failed to build test servers: %v", err)
	}
	RunSpecs(t, "Tablecloth")
}

func buildTestServers() error {
	cmd := exec.Command("make", "-B")
	cmd.Dir = "./test_servers"
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%v\n%s", err, output)
	}
	return nil
}

var _ = Describe("Tablecloth HTTP listener", func() {
	var (
		serverCmd *exec.Cmd
	)

	AfterEach(func() {
		stopServer(serverCmd)
	})

	Context("Listening on a single port", func() {

		BeforeEach(func() {
			serverCmd = startServer("simple_server", "-listenAddr=127.0.0.1:8081")
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
			resultCh := startVegetaAttack([]string{"GET http://127.0.0.1:8081"}, 40, 3*time.Second)

			time.Sleep(100 * time.Millisecond)
			reloadServer(serverCmd)

			metrics := <-resultCh
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
			resultCh := startVegetaAttack([]string{"GET http://127.0.0.1:8081", "GET http://127.0.0.1:8082"}, 40, 3*time.Second)

			time.Sleep(100 * time.Millisecond)
			reloadServer(serverCmd)

			metrics := <-resultCh
			Expect(metrics.StatusCodes["200"]).To(Equal(int(metrics.Requests)))
		})

	})

	It("should still restart if connections haven't closed within the timeout", func() {
		// Start with a closeTimeout of 100ms (which is less that the response time of 250ms)
		serverCmd = startServer("simple_server", "-listenAddr=127.0.0.1:8081", "-closeTimeout=100ms")
		parentPid := serverCmd.Process.Pid

		go http.Get("http://127.0.0.1:8081/")
		time.Sleep(10 * time.Millisecond)
		reloadServer(serverCmd)

		resp, err := http.Get("http://127.0.0.1:8081/")
		Expect(err).To(BeNil())
		newBody, _ := ioutil.ReadAll(resp.Body)
		resp.Body.Close()

		Expect(resp.StatusCode).To(Equal(200))

		Expect(string(newBody)).To(ContainSubstring("Hello (pid=%d)", parentPid))
	})

	Describe("changing the working directory", func() {
		var (
			cwd string
		)
		BeforeEach(func() {
			cwd, _ = os.Getwd()
		})
		AfterEach(func() {
			os.Chdir(cwd)
			os.Remove(cwd + "/test_servers/current")
		})
		It("should change the working directory before re-execing", func() {
			os.Chdir(cwd + "/test_servers/v1")

			serverCmd = startServer("./server", "-workingDir="+cwd+"/test_servers/v2")
			parentPid := serverCmd.Process.Pid

			resp, err := http.Get("http://127.0.0.1:8081/")
			Expect(err).To(BeNil())
			body, _ := ioutil.ReadAll(resp.Body)

			Expect(string(body)).To(ContainSubstring("Hello from v1 (pid=%d)", parentPid))

			reloadServer(serverCmd)

			resp, err = http.Get("http://127.0.0.1:8081/")
			Expect(err).To(BeNil())
			body, _ = ioutil.ReadAll(resp.Body)

			Expect(string(body)).To(ContainSubstring("Hello from v2 (pid=%d)", parentPid))
		})

		It("should work with a working directory that's a symlink", func() {
			err := os.Symlink(cwd+"/test_servers/v1", cwd+"/test_servers/current")
			Expect(err).To(BeNil())

			os.Chdir(cwd + "/test_servers/current")

			serverCmd = startServer("./server", "-workingDir="+cwd+"/test_servers/current")
			parentPid := serverCmd.Process.Pid

			resp, err := http.Get("http://127.0.0.1:8081/")
			Expect(err).To(BeNil())
			body, _ := ioutil.ReadAll(resp.Body)

			Expect(string(body)).To(ContainSubstring("Hello from v1 (pid=%d)", parentPid))

			err = os.Remove(cwd + "/test_servers/current")
			Expect(err).To(BeNil())
			err = os.Symlink(cwd+"/test_servers/v2", cwd+"/test_servers/current")
			Expect(err).To(BeNil())

			reloadServer(serverCmd)

			resp, err = http.Get("http://127.0.0.1:8081/")
			Expect(err).To(BeNil())
			body, _ = ioutil.ReadAll(resp.Body)

			Expect(string(body)).To(ContainSubstring("Hello from v2 (pid=%d)", parentPid))
		})

	})
})

func startServer(server string, args ...string) (cmd *exec.Cmd) {
	if !strings.HasPrefix(server, "./") {
		server = fmt.Sprintf("./test_servers/%s", server)
	}
	cmd = exec.Command(server, args...)
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
