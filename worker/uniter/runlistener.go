// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// The run listener is a worker go-routine that opens listens on a free high
// port for juju-run commands.

package uniter

import (
	"net"
	"net/rpc"
	"sync"

	"launchpad.net/juju-core/utils/fslock"
)

type RunListener struct {
	listener net.Listener
	server   *rpc.Server
	hookLock *fslock.Lock
	closed   chan bool
	closing  chan bool
	wg       sync.WaitGroup
}

type RunResults struct {
	StdOut     string
	StdErr     string
	ReturnCode int
}

type Runner struct{}

func (*Runner) RunCommands(commands string, result *RunResults) error {
	// TODO: grab the hook flock, and pipe the commands through /bin/bash -s
	logger.Debugf("RunCommands: %q")
	result = &RunResults{
		StdOut:     "stdout",
		StdErr:     "stderr",
		ReturnCode: 0,
	}
	return nil
}

func NewRunListener(hookLock *fslock.Lock, netType, localAddr string) (*RunListener, error) {
	server := rpc.NewServer()
	if err := server.Register(&Runner{}); err != nil {
		return nil, err
	}
	listener, err := net.Listen(netType, localAddr)
	if err != nil {
		logger.Errorf("failed to listen on %s %s: %v", netType, localAddr, err)
		return nil, err
	}
	runner := &RunListener{
		listener: listener,
		server:   server,
		hookLock: hookLock,
		closed:   make(chan bool),
		closing:  make(chan bool),
	}
	return runner, nil
}

// Run accepts new connections until it encounters an error, or until Close is
// called, and then blocks until all existing connections have been closed.
func (s *RunListener) Run() (err error) {
	var conn net.Conn
	for {
		conn, err = s.listener.Accept()
		if err != nil {
			break
		}
		s.wg.Add(1)
		go func(conn net.Conn) {
			s.server.ServeConn(conn)
			s.wg.Done()
		}(conn)
	}
	select {
	case <-s.closing:
		// Someone has called Close(), so it is overwhelmingly likely that
		// the error from Accept is a direct result of the Listener being
		// closed, and can therefore be safely ignored.
		err = nil
	default:
	}
	s.wg.Wait()
	close(s.closed)
	return
}

// Close immediately stops accepting connections, and blocks until all existing
// connections have been closed.
func (s *RunListener) Close() {
	close(s.closing)
	s.listener.Close()
	<-s.closed
}
