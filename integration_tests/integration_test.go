package integration_test

import (
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/phayes/freeport"
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
		serverCmd  *exec.Cmd
		serverAddr string
	)

	AfterEach(func() {
		stopServer(serverCmd)
	})

	Context("Listening on a single port", func() {

		BeforeEach(func() {
			serverCmd, serverAddr = startServer("simple/server")
		})

		It("Should listen on the given address", func() {
			resp, err := http.Get("http://" + serverAddr + "/")
			Expect(err).To(BeNil())

			Expect(resp.StatusCode).To(Equal(200))
		})

		It("Should restart when given a HUP signal", func() {
			resp, err := http.Get("http://" + serverAddr + "/")
			Expect(err).To(BeNil())
			firstBody, _ := ioutil.ReadAll(resp.Body)
			resp.Body.Close()

			Expect(resp.StatusCode).To(Equal(200))

			reloadServer(serverCmd)

			resp, err = http.Get("http://" + serverAddr + "/")
			Expect(err).To(BeNil())
			newBody, _ := ioutil.ReadAll(resp.Body)
			resp.Body.Close()

			Expect(resp.StatusCode).To(Equal(200))

			// The response body includes the start time of the server
			Expect(string(newBody)).NotTo(Equal(string(firstBody)))
		})

		It("Should not drop any requests while reloading", func() {
			resultCh := startVegetaAttack([]string{"GET http://" + serverAddr}, 40, 3*time.Second)

			time.Sleep(100 * time.Millisecond)
			reloadServer(serverCmd)

			metrics := <-resultCh
			Expect(metrics.StatusCodes["200"]).To(Equal(int(metrics.Requests)))
		})

		if canReadProcessFds() {
			It("should not leak file descriptors when reloading", func() {
				resp, err := http.Get("http://" + serverAddr + "/")
				Expect(err).NotTo(HaveOccurred())
				Expect(resp.StatusCode).To(Equal(200))

				initalFds := getProcessFds(serverCmd)

				reloadServer(serverCmd)

				resp, err = http.Get("http://" + serverAddr + "/")
				Expect(err).NotTo(HaveOccurred())
				Expect(resp.StatusCode).To(Equal(200))

				currentFds := getProcessFds(serverCmd)
				Expect(currentFds).To(Equal(initalFds))
			})
		} else {
			PIt("leaking file descriptors test requires /proc/<pid>/fd directories")
		}
	})

	Context("Listening on multiple ports", func() {
		var (
			serverAddr2 string
		)

		BeforeEach(func() {
			serverCmd, serverAddr, serverAddr2 = startServerDouble("double_listen/server")
		})

		It("Should listen on the given addresses", func() {
			resp, err := http.Get("http://" + serverAddr + "/")
			Expect(err).To(BeNil())

			Expect(resp.StatusCode).To(Equal(200))

			resp, err = http.Get("http://" + serverAddr2 + "/")
			Expect(err).To(BeNil())

			Expect(resp.StatusCode).To(Equal(200))
		})

		It("Should restart when given a HUP signal", func() {
			resp, err := http.Get("http://" + serverAddr + "/")
			Expect(err).To(BeNil())
			firstBody1, _ := ioutil.ReadAll(resp.Body)
			resp.Body.Close()

			Expect(resp.StatusCode).To(Equal(200))

			resp, err = http.Get("http://" + serverAddr2 + "/")
			Expect(err).To(BeNil())
			firstBody2, _ := ioutil.ReadAll(resp.Body)
			resp.Body.Close()

			Expect(resp.StatusCode).To(Equal(200))

			reloadServer(serverCmd)

			resp, err = http.Get("http://" + serverAddr + "/")
			Expect(err).To(BeNil())
			newBody, _ := ioutil.ReadAll(resp.Body)
			resp.Body.Close()

			Expect(resp.StatusCode).To(Equal(200))

			// The response body includes the start time of the server
			Expect(string(newBody)).NotTo(Equal(string(firstBody1)))

			resp, err = http.Get("http://" + serverAddr2 + "/")
			Expect(err).To(BeNil())
			newBody, _ = ioutil.ReadAll(resp.Body)
			resp.Body.Close()

			Expect(resp.StatusCode).To(Equal(200))

			// The response body includes the start time of the server
			Expect(string(newBody)).NotTo(Equal(string(firstBody2)))
		})

		It("Should not drop any requests while reloading", func() {
			resultCh := startVegetaAttack([]string{"GET http://" + serverAddr, "GET http://" + serverAddr2}, 40, 3*time.Second)

			time.Sleep(100 * time.Millisecond)
			reloadServer(serverCmd)

			metrics := <-resultCh
			Expect(metrics.StatusCodes["200"]).To(Equal(int(metrics.Requests)))
		})

	})

	It("should still restart if connections haven't closed within the timeout", func() {
		// Start with a closeTimeout of 100ms (which is less that the response time of 250ms)
		serverCmd, serverAddr = startServer("simple/server", "-closeTimeout=100ms")
		parentPid := serverCmd.Process.Pid

		go http.Get("http://" + serverAddr + "/")
		time.Sleep(10 * time.Millisecond)
		reloadServer(serverCmd)

		resp, err := http.Get("http://" + serverAddr + "/")
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

			serverCmd, serverAddr = startServer("./server", "-workingDir="+cwd+"/test_servers/v2")
			parentPid := serverCmd.Process.Pid

			resp, err := http.Get("http://" + serverAddr + "/")
			Expect(err).To(BeNil())
			body, _ := ioutil.ReadAll(resp.Body)

			Expect(string(body)).To(ContainSubstring("Hello from v1 (pid=%d)", parentPid))

			reloadServer(serverCmd)

			resp, err = http.Get("http://" + serverAddr + "/")
			Expect(err).To(BeNil())
			body, _ = ioutil.ReadAll(resp.Body)

			Expect(string(body)).To(ContainSubstring("Hello from v2 (pid=%d)", parentPid))
		})

		It("should work with a working directory that's a symlink", func() {
			err := os.Symlink(cwd+"/test_servers/v1", cwd+"/test_servers/current")
			Expect(err).To(BeNil())

			os.Chdir(cwd + "/test_servers/current")

			serverCmd, serverAddr = startServer("./server", "-workingDir="+cwd+"/test_servers/current")
			parentPid := serverCmd.Process.Pid

			resp, err := http.Get("http://" + serverAddr + "/")
			Expect(err).To(BeNil())
			body, _ := ioutil.ReadAll(resp.Body)

			Expect(string(body)).To(ContainSubstring("Hello from v1 (pid=%d)", parentPid))

			err = os.Remove(cwd + "/test_servers/current")
			Expect(err).To(BeNil())
			err = os.Symlink(cwd+"/test_servers/v2", cwd+"/test_servers/current")
			Expect(err).To(BeNil())

			reloadServer(serverCmd)

			resp, err = http.Get("http://" + serverAddr + "/")
			Expect(err).To(BeNil())
			body, _ = ioutil.ReadAll(resp.Body)

			Expect(string(body)).To(ContainSubstring("Hello from v2 (pid=%d)", parentPid))
		})

	})

	Describe("handling failures starting the new version", func() {
		var cwd string
		BeforeEach(func() {
			cwd, _ = os.Getwd()
			Expect(os.Symlink(cwd+"/test_servers/v1", cwd+"/test_servers/current")).To(Succeed())
			Expect(os.Chdir(cwd + "/test_servers/current")).To(Succeed())
			serverCmd, serverAddr = startServer("./server", "-workingDir="+cwd+"/test_servers/current")
		})
		AfterEach(func() {
			os.Chdir(cwd)
			os.Remove(cwd + "/test_servers/current")
		})

		Context("the new server fails to start", func() {
			It("should continue running the old server", func() {
				resp, err := http.Get("http://" + serverAddr + "/")
				Expect(err).NotTo(HaveOccurred())
				Expect(resp.StatusCode).To(Equal(200))
				firstBody, _ := ioutil.ReadAll(resp.Body)
				resp.Body.Close()

				err = os.Remove(cwd + "/test_servers/current")
				Expect(err).NotTo(HaveOccurred())

				// Non-existent directory will cause starting the server to error
				err = os.Symlink(cwd+"/test_servers/non_existent", cwd+"/test_servers/current")
				Expect(err).NotTo(HaveOccurred())

				withSilentOutput(func() {
					reloadServer(serverCmd)
				})

				resp, err = http.Get("http://" + serverAddr + "/")
				Expect(err).NotTo(HaveOccurred())
				Expect(resp.StatusCode).To(Equal(200))
				newBody, _ := ioutil.ReadAll(resp.Body)
				resp.Body.Close()

				Expect(newBody).To(Equal(firstBody))
			})

			if canReadProcessFds() {
				It("should not leak file descriptors when reloading fails", func() {
					resp, err := http.Get("http://" + serverAddr + "/")
					Expect(err).NotTo(HaveOccurred())
					Expect(resp.StatusCode).To(Equal(200))

					initalFds := getProcessFds(serverCmd)

					Expect(os.Remove(cwd + "/test_servers/current")).To(Succeed())
					// Non-existent directory will cause starting the server to error
					Expect(os.Symlink(cwd+"/test_servers/non_existent", cwd+"/test_servers/current")).To(Succeed())

					withSilentOutput(func() {
						reloadServer(serverCmd)
					})

					resp, err = http.Get("http://" + serverAddr + "/")
					Expect(err).NotTo(HaveOccurred())
					Expect(resp.StatusCode).To(Equal(200))

					currentFds := getProcessFds(serverCmd)
					Expect(currentFds).To(Equal(initalFds))
				})
			} else {
				PIt("leaking file descriptors test requires /proc/<pid>/fd directories")
			}

			It("should successfully handle subsequent reload requests with a good server", func() {
				resp, err := http.Get("http://" + serverAddr + "/")
				Expect(err).NotTo(HaveOccurred())
				Expect(resp.StatusCode).To(Equal(200))

				err = os.Remove(cwd + "/test_servers/current")
				Expect(err).NotTo(HaveOccurred())
				// Non-existent directory will cause starting the server to error
				err = os.Symlink(cwd+"/test_servers/non_existent", cwd+"/test_servers/current")
				Expect(err).NotTo(HaveOccurred())

				// Attempt reload with broken server
				withSilentOutput(func() {
					reloadServer(serverCmd)
				})
				resp, err = http.Get("http://" + serverAddr + "/")
				Expect(err).NotTo(HaveOccurred())
				Expect(resp.StatusCode).To(Equal(200))

				// Now point back at a good server
				Expect(os.Remove(cwd + "/test_servers/current")).To(Succeed())
				Expect(os.Symlink(cwd+"/test_servers/v2", cwd+"/test_servers/current")).To(Succeed())

				reloadServer(serverCmd)

				resp, err = http.Get("http://" + serverAddr + "/")
				Expect(err).NotTo(HaveOccurred())
				Expect(resp.StatusCode).To(Equal(200))

				body, _ := ioutil.ReadAll(resp.Body)
				Expect(string(body)).To(ContainSubstring("Hello from v2"))
			})
		})

		Context("the new server exits shortly after starting", func() {
			It("should continue running the old server", func() {
				resp, err := http.Get("http://" + serverAddr + "/")
				Expect(err).NotTo(HaveOccurred())
				Expect(resp.StatusCode).To(Equal(200))
				firstBody, _ := ioutil.ReadAll(resp.Body)
				resp.Body.Close()

				Expect(os.Remove(cwd + "/test_servers/current")).To(Succeed())
				Expect(os.Symlink(cwd+"/test_servers/errorer", cwd+"/test_servers/current")).To(Succeed())

				withSilentOutput(func() {
					reloadServer(serverCmd)
				})

				resp, err = http.Get("http://" + serverAddr + "/")
				Expect(err).NotTo(HaveOccurred())
				Expect(resp.StatusCode).To(Equal(200))
				newBody, _ := ioutil.ReadAll(resp.Body)
				resp.Body.Close()

				Expect(newBody).To(Equal(firstBody))
			})

			if canReadProcessFds() {
				It("should not leak file descriptors when reloading fails", func() {
					resp, err := http.Get("http://" + serverAddr + "/")
					Expect(err).NotTo(HaveOccurred())
					Expect(resp.StatusCode).To(Equal(200))

					initalFds := getProcessFds(serverCmd)

					Expect(os.Remove(cwd + "/test_servers/current")).To(Succeed())
					Expect(os.Symlink(cwd+"/test_servers/errorer", cwd+"/test_servers/current")).To(Succeed())

					withSilentOutput(func() {
						reloadServer(serverCmd)
					})

					resp, err = http.Get("http://" + serverAddr + "/")
					Expect(err).NotTo(HaveOccurred())
					Expect(resp.StatusCode).To(Equal(200))

					currentFds := getProcessFds(serverCmd)
					Expect(currentFds).To(Equal(initalFds))
				})
			} else {
				PIt("leaking file descriptors test requires /proc/<pid>/fd directories")
			}

			It("should successfully handle subsequent reload requests with a good server", func() {
				resp, err := http.Get("http://" + serverAddr + "/")
				Expect(err).NotTo(HaveOccurred())
				Expect(resp.StatusCode).To(Equal(200))

				Expect(os.Remove(cwd + "/test_servers/current")).To(Succeed())
				Expect(os.Symlink(cwd+"/test_servers/errorer", cwd+"/test_servers/current")).To(Succeed())

				withSilentOutput(func() {
					reloadServer(serverCmd)
				})
				resp, err = http.Get("http://" + serverAddr + "/")
				Expect(err).NotTo(HaveOccurred())
				Expect(resp.StatusCode).To(Equal(200))

				// Now point back at a good server
				Expect(os.Remove(cwd + "/test_servers/current")).To(Succeed())
				Expect(os.Symlink(cwd+"/test_servers/v2", cwd+"/test_servers/current")).To(Succeed())

				reloadServer(serverCmd)

				resp, err = http.Get("http://" + serverAddr + "/")
				Expect(err).NotTo(HaveOccurred())
				Expect(resp.StatusCode).To(Equal(200))

				body, _ := ioutil.ReadAll(resp.Body)
				Expect(string(body)).To(ContainSubstring("Hello from v2"))
			})
		})
	})
})

var serverOutputWriter = &struct{ io.Writer }{os.Stderr}

func withSilentOutput(f func()) {
	serverOutputWriter.Writer = ioutil.Discard
	defer func() { serverOutputWriter.Writer = os.Stderr }()
	f()
}

func startServer(server string, args ...string) (*exec.Cmd, string) {
	port, err := freeport.GetFreePort()
	ExpectWithOffset(1, err).NotTo(HaveOccurred())
	addr := "127.0.0.1:" + strconv.Itoa(port)
	cmd := execServer(server, append(args, "-listenAddr="+addr)...)
	return cmd, addr
}

func startServerDouble(server string, args ...string) (*exec.Cmd, string, string) {
	ports, err := freeport.GetFreePorts(2)
	ExpectWithOffset(1, err).NotTo(HaveOccurred())
	addr1 := "127.0.0.1:" + strconv.Itoa(ports[0])
	addr2 := "127.0.0.1:" + strconv.Itoa(ports[1])
	cmd := execServer(server, append(args, "-listenAddr1="+addr1, "-listenAddr2="+addr2)...)
	return cmd, addr1, addr2
}

func execServer(server string, args ...string) *exec.Cmd {
	if !strings.HasPrefix(server, "./") {
		server = fmt.Sprintf("./test_servers/%s", server)
	}
	cmd := exec.Command(server, args...)
	cmd.Stdout = serverOutputWriter
	cmd.Stderr = serverOutputWriter
	err := cmd.Start()
	ExpectWithOffset(2, err).NotTo(HaveOccurred())
	time.Sleep(50 * time.Millisecond)

	return cmd
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

func canReadProcessFds() bool {
	fi, err := os.Stat(fmt.Sprintf("/proc/%d/fd", os.Getpid()))
	if err != nil {
		return false
	}
	return fi.IsDir()
}

func getProcessFds(cmd *exec.Cmd) []string {
	// Ensure any http request sockets have closed
	time.Sleep(10 * time.Millisecond)

	dir := fmt.Sprintf("/proc/%d/fd", cmd.Process.Pid)
	fileInfos, err := ioutil.ReadDir(dir)
	ExpectWithOffset(1, err).NotTo(HaveOccurred())
	fds := make([]string, 0, len(fileInfos))
	for _, fi := range fileInfos {
		target, err := os.Readlink(dir + "/" + fi.Name())
		ExpectWithOffset(1, err).NotTo(HaveOccurred())
		fds = append(fds, fi.Name()+"->"+target)
	}
	return fds
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
