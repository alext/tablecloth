package upgradeable_http

import (
	"errors"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"strconv"
	"sync"
	"syscall"
	"time"
)

var (
	StartupDelay     = 5 * time.Second
	CloseWaitTimeout = 30 * time.Second
	theManager       = &manager{}

	// variable indirection to facilitate testing
	setupFunc = theManager.setup
)

func ListenAndServe(addr string, handler http.Handler, idents ...string) error {
	theManager.once.Do(setupFunc)

	ident := "default"
	if len(idents) >= 1 {
		ident = idents[0]
	}

	return theManager.listenAndServe(addr, handler, ident)
}

type manager struct {
	once            sync.Once
	listeners       map[string]*gracefulListener
	listenersLock   sync.Mutex
	activeListeners sync.WaitGroup
	inParent        bool
}

func (m *manager) setup() {
	m.listeners = make(map[string]*gracefulListener)
	m.inParent = os.Getenv("TEMPORARY_CHILD") != "1"

	go m.handleSignals()

	if m.inParent {
		go m.stopTemporaryChild()
	}
}

func (m *manager) listenAndServe(addr string, handler http.Handler, ident string) error {
	m.activeListeners.Add(1)
	defer m.activeListeners.Done()

	l, err := m.setupListener(addr, ident)
	if err != nil {
		return err
	}

	err = http.Serve(l, handler)
	if l.stopping {
		err = l.waitForClients(CloseWaitTimeout)
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

func (m *manager) setupListener(addr, ident string) (l *gracefulListener, err error) {
	m.listenersLock.Lock()
	defer m.listenersLock.Unlock()

	if m.listeners[ident] != nil {
		return nil, errors.New("duplicate ident")
	}

	l, err = resumeOrListen(listenFdFromEnv(ident), addr)
	if err != nil {
		return nil, err
	}
	m.listeners[ident] = l
	return
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

	time.Sleep(StartupDelay)

	fds := make(map[string]int, len(m.listeners))
	for ident, l := range m.listeners {
		fd, err := l.prepareFd()
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
		fd, err := l.prepareFd()
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
