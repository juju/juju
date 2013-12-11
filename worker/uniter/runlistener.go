// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// The run listener is a worker go-routine that opens listens on a free high
// port for juju-run commands.

package uniter

import (
	"net"
	"net/rpc"
	"sync"
)

type CommandRunner interface {
	RunCommands(commands string) (results *RunResults, err error)
}

type RunListener struct {
	listener net.Listener
	server   *rpc.Server
	closed   chan bool
	closing  chan bool
	wg       sync.WaitGroup
}

type RunResults struct {
	StdOut     string
	StdErr     string
	ReturnCode int
}

type Runner struct {
	runner CommandRunner
}

func (r *Runner) RunCommands(commands string, result *RunResults) error {
	logger.Debugf("RunCommands: %q", commands)
	runResult, err := r.runner.RunCommands(commands)
	*result = *runResult
	return err
}

func NewRunListener(runner CommandRunner, netType, localAddr string) (*RunListener, error) {
	server := rpc.NewServer()
	if err := server.Register(&Runner{runner}); err != nil {
		return nil, err
	}
	listener, err := net.Listen(netType, localAddr)
	if err != nil {
		logger.Errorf("failed to listen on %s %s: %v", netType, localAddr, err)
		return nil, err
	}
	runListener := &RunListener{
		listener: listener,
		server:   server,
		closed:   make(chan bool),
		closing:  make(chan bool),
	}
	go runListener.Run()
	return runListener, nil
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
