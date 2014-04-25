package upgradeable_http

import (
	"fmt"
	"net"
	"os"
	"sync/atomic"
	"syscall"
	"time"
)

type watchedConn struct {
	net.Conn
	listener *GracefulListener
}

func (c *watchedConn) Close() (err error) {
	err = c.Conn.Close()
	c.listener.decCount()
	return
}

func ResumeOrListen(fd int, addr string) (*GracefulListener, error) {
	var l net.Listener
	var err error
	if fd != 0 {
		f := os.NewFile(uintptr(fd), "listen socket")
		l, err = net.FileListener(f)
		e := f.Close()
		if e != nil {
			return nil, e
		}
	} else {
		l, err = net.Listen("tcp", addr)
	}
	if err != nil {
		return nil, err
	}

	return &GracefulListener{Listener: l}, nil
}

type GracefulListener struct {
	net.Listener
	connCount int64
	stopping  bool
}

func (l *GracefulListener) Addr() (a net.Addr) {
	tcpListener, ok := l.Listener.(*net.TCPListener)
	if ok {
		return tcpListener.Addr()
	}
	return nil
}

func (l *GracefulListener) Accept() (c net.Conn, err error) {
	c, err = l.Listener.Accept()
	if err != nil {
		return
	}
	c = &watchedConn{Conn: c, listener: l}
	l.incCount()
	return
}

func (l *GracefulListener) Close() error {
	l.stopping = true
	return l.Listener.Close()
}

func (l *GracefulListener) Stopping() bool {
	return l.stopping
}

func (l *GracefulListener) getCount() int64 {
	return atomic.LoadInt64(&l.connCount)
}
func (l *GracefulListener) incCount() {
	atomic.AddInt64(&l.connCount, 1)
}
func (l *GracefulListener) decCount() {
	atomic.AddInt64(&l.connCount, -1)
}

func (l *GracefulListener) WaitForClients(timeout int) error {
	for i := 0; i < timeout; i++ {
		if l.getCount() == 0 {
			return nil
		}
		time.Sleep(1 * time.Second)
	}
	if l.getCount() == 0 {
		return nil
	}
	return fmt.Errorf("Still %d active clients after %d seconds", l.getCount(), timeout)
}

func (l *GracefulListener) PrepareFd() (fd int, err error) {
	tl := l.Listener.(*net.TCPListener)
	fl, err := tl.File()
	if err != nil {
		return 0, err
	}

	// Dup the fd to clear the CloseOnExec flag
	fd, err = syscall.Dup(int(fl.Fd()))
	if err != nil {
		return 0, err
	}
	return
}
