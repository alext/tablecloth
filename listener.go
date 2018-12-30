package tablecloth

import (
	"errors"
	"net"
	"os"
	"syscall"
)

func resumeOrListen(fd int, addr string) (*net.TCPListener, error) {
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
	tl, ok := l.(*net.TCPListener)
	if !ok {
		return nil, errors.New("passed file descriptor is not for a TCP socket")
	}
	return tl, nil
}

func prepareListenerFd(tl *net.TCPListener) (fd int, err error) {
	fl, err := tl.File()
	if err != nil {
		return 0, err
	}
	defer fl.Close()

	// The TCPListener.File() sets the underlying socket to be blocking
	// (http://git.io/veIh6).  This alters the behaviour of Accept such that
	// when the listener fd is closed, Accept doesn't return an error until the
	// next connection comes in.
	//
	// Setting this back to non-blocking allows this to continue to use the
	// epoll mechanism meaning that Accept will return an error immediately
	// when the listener fd is closed.
	syscall.SetNonblock(int(fl.Fd()), true)

	// Dup the fd to clear the CloseOnExec flag
	fd, err = syscall.Dup(int(fl.Fd()))
	if err != nil {
		return 0, err
	}
	return fd, nil
}
