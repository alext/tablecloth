package upgradeable_http

import (
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

var (
	StartupDelay = 1 * time.Second
	CloseWaitTimeout = 30 * time.Second
)

type Manager interface {
	ListenAndServe(ident, addr string, handler http.Handler) error
}

type manager struct {
	listeners       map[string]*GracefulListener
	listenersLock   sync.Mutex
	activeListeners sync.WaitGroup
	inParent        bool
}

func NewManager() (m *manager) {
	m = &manager{}
	m.listeners = make(map[string]*GracefulListener)
	m.inParent = os.Getenv("TEMPORARY_CHILD") != "1"

	go m.handleSignals()

	if m.inParent {
		go m.stopTemporaryChild()
	}
	return
}

func (m *manager) ListenAndServe(ident, addr string, handler http.Handler) error {
	m.activeListeners.Add(1)
	defer m.activeListeners.Done()

	l, err := ResumeOrListen(listenFdFromEnv(ident), addr)
	if err != nil {
		return err
	}

	m.listenersLock.Lock()
	m.listeners[ident] = l
	m.listenersLock.Unlock()

	err = http.Serve(l, handler)
	if l.Stopping() {
		err = l.WaitForClients(CloseWaitTimeout)
		if m.inParent {
			// TODO: notify/log WaitForClients errors somehow.

			// This function will now never return, so the above defer won't happen.
			m.activeListeners.Done()

			// prevent this goroutine returning before the server has re-exec'd
			// This is to cover the case where this is the main goroutine, and exiting
			// would therefore prevent the re-exec happening
			c := make(chan bool)
			<-c
		} else if err != nil {
			return err
		}
	} else if err != nil {
		return err
	}
	return nil
}

func listenFdFromEnv(ident string) int {
	listenFD, err := strconv.Atoi(os.Getenv("LISTEN_FD_" + ident))
	if err != nil {
		return 0
	}
	return listenFD
}

func (m *manager) handleSignals() {
	c := make(chan os.Signal, 1)
	signal.Notify(c, syscall.SIGHUP)
	_ = <-c

	m.listenersLock.Lock()
	defer m.listenersLock.Unlock()

	if m.inParent {
		m.upgradeServer()
	}

	m.closeListeners()
}

func (m *manager) upgradeServer() {
	proc, err := m.startTemporaryChild()
	if err != nil {
		// TODO: better error handling
		panic(err)
	}

	// TODO: Better means of waiting for child to start serving
	time.Sleep(StartupDelay)

	fds := make(map[string]int, len(m.listeners))
	for ident, l := range m.listeners {
		fd, err := l.PrepareFd()
		if err != nil {
			panic(err)
			// TODO: better error handling
		}
		fds[ident] = fd
	}

	go m.reExecSelf(fds, proc.Pid)
}

func (m *manager) closeListeners() {
	for _, l := range m.listeners {
		l.Close()
	}
}

func (m *manager) reExecSelf(fds map[string]int, childPid int) {
	// wait until there are no active listeners
	m.activeListeners.Wait()

	em := newEnvMap(os.Environ())
	for ident, fd := range fds {
		em["LISTEN_FD_"+ident] = strconv.Itoa(fd)
	}
	em["TEMPORARY_CHILD_PID"] = strconv.Itoa(childPid)

	syscall.Exec(os.Args[0], os.Args, em.ToEnv())
}

func (m *manager) startTemporaryChild() (proc *os.Process, err error) {

	cmd := exec.Command(os.Args[0], os.Args[1:]...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	em := newEnvMap(os.Environ())
	for ident, l := range m.listeners {
		fd, err := l.PrepareFd()
		if err != nil {
			return nil, err
		}
		em["LISTEN_FD_"+ident] = strconv.Itoa(fd)
	}
	em["TEMPORARY_CHILD"] = "1"
	cmd.Env = em.ToEnv()

	err = cmd.Start()
	if err != nil {
		return nil, err
	}
	return cmd.Process, nil
}

func (m *manager) stopTemporaryChild() {
	childPid, err := strconv.Atoi(os.Getenv("TEMPORARY_CHILD_PID"))
	if err != nil {
		// non-integer/blank TEMPORARY_CHILD_PID so ignore
		return
	}

	// TODO: Better meand of waiting for parent to start
	time.Sleep(StartupDelay)

	proc, err := os.FindProcess(childPid)
	if err != nil {
		//TODO: something better here?
		// Failed to find process
		return
	}
	err = proc.Signal(syscall.SIGHUP)
	if err != nil {
		//TODO: better error handling
		return
	}
	_, err = proc.Wait()
	if err != nil {
		//TODO: better error handling
		return
	}
}

type envMap map[string]string

func newEnvMap(env []string) (em envMap) {
	em = make(map[string]string, len(env))
	for _, item := range env {
		parts := strings.SplitN(item, "=", 2)
		em[parts[0]] = parts[1]
	}
	return
}

func (em envMap) ToEnv() (env []string) {
	env = make([]string, 0, len(em))
	for k, v := range em {
		env = append(env, k+"="+v)
	}
	return
}
