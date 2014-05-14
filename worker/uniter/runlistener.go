// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// The run listener is a worker go-routine that listens on either a unix
// socket or a tcp connection for juju-run commands.

package uniter

import (
	"net"
	"net/rpc"
	"os"
	"sync"

	"launchpad.net/juju-core/utils/exec"
)

const JujuRunEndpoint = "JujuRunServer.RunCommands"

// A CommandRunner is something that will actually execute the commands and
// return the results of that execution in the exec.ExecResponse (which
// contains stdout, stderr, and return code).
type CommandRunner interface {
	RunCommands(commands string) (results *exec.ExecResponse, err error)
}

// RunListener is responsible for listening on the network connection and
// seting up the rpc server on that net connection. Also starts the go routine
// that listens and hands off the work.
type RunListener struct {
	listener net.Listener
	server   *rpc.Server
	closed   chan struct{}
	closing  chan struct{}
	wg       sync.WaitGroup
}

// The JujuRunServer is the entity that has the methods that are called over
// the rpc connection.
type JujuRunServer struct {
	runner CommandRunner
}

// RunCommands delegates the actual running to the runner and populates the
// response structure.
func (r *JujuRunServer) RunCommands(commands string, result *exec.ExecResponse) error {
	logger.Debugf("RunCommands: %q", commands)
	runResult, err := r.runner.RunCommands(commands)
	*result = *runResult
	return err
}

// NewRunListener returns a new RunListener that is listening on given
// unix socket path passed in. If a valid RunListener is returned, is
// has the go routine running, and should be closed by the creator
// when they are done with it.
func NewRunListener(runner CommandRunner, socketPath string) (*RunListener, error) {
	server := rpc.NewServer()
	if err := server.Register(&JujuRunServer{runner}); err != nil {
		return nil, err
	}
	// In case the unix socket is present, delete it.
	if err := os.Remove(socketPath); err != nil {
		logger.Tracef("ignoring error on removing %q: %v", socketPath, err)
	}
	listener, err := net.Listen("unix", socketPath)
	if err != nil {
		logger.Errorf("failed to listen on unix:%s: %v", socketPath, err)
		return nil, err
	}
	runListener := &RunListener{
		listener: listener,
		server:   server,
		closed:   make(chan struct{}),
		closing:  make(chan struct{}),
	}
	go runListener.Run()
	return runListener, nil
}

// Run accepts new connections until it encounters an error, or until Close is
// called, and then blocks until all existing connections have been closed.
func (s *RunListener) Run() (err error) {
	logger.Debugf("juju-run listener running")
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
	logger.Debugf("juju-run listener stopping")
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
	logger.Debugf("juju-run listener stopped")
}
