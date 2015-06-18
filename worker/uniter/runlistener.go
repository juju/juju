// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// The run listener is a worker go-routine that listens on either a unix
// socket or a tcp connection for juju-run commands.

package uniter

import (
	"net"
	"sync"

	"github.com/juju/errors"
	"github.com/juju/utils/exec"

	"github.com/juju/juju/juju/sockets"
	"github.com/juju/juju/rpc"
	"github.com/juju/juju/rpc/jsoncodec"
)

const JujuRunServerType = "JujuRunServer"
const JujuRunRunCommandsAction = "RunCommands"

// RunCommandsArgs stores the arguments for a RunCommands call.
type RunCommandsArgs struct {
	// Commands is the arbitrary commands to execute on the unit
	Commands string
	// RelationId is the relation context to execute the commands in.
	RelationId int
	// RemoteUnitName is the remote unit for the relation context.
	RemoteUnitName string
	// ForceRemoteUnit skips relation membership and existence validation.
	ForceRemoteUnit bool
}

// A CommandRunner is something that will actually execute the commands and
// return the results of that execution in the exec.ExecResponse (which
// contains stdout, stderr, and return code).
type CommandRunner interface {
	RunCommands(RunCommandsArgs RunCommandsArgs) (results *exec.ExecResponse, err error)
}

// RunListener is responsible for listening on the network connection and
// seting up the rpc server on that net connection. Also starts the go routine
// that listens and hands off the work.
type RunListener struct {
	listener net.Listener
	srvRoot  *jujuRunRpcRoot
	closed   chan struct{}
	closing  chan struct{}
	wg       sync.WaitGroup
}

type jujuRunRpcRoot struct {
	runner CommandRunner
}

func (r *jujuRunRpcRoot) JujuRunServer(id string) (*JujuRunServer, error) {
	return &JujuRunServer{r.runner}, nil
}

// The JujuRunServer is the entity that has the methods that are called over
// the rpc connection.
type JujuRunServer struct {
	runner CommandRunner
}

// RunCommands delegates the actual running to the runner and populates the
// response structure.
func (r *JujuRunServer) RunCommands(args RunCommandsArgs) (exec.ExecResponse, error) {
	logger.Debugf("RunCommands: %+v", args)
	runResult, err := r.runner.RunCommands(args)
	if err != nil {
		return exec.ExecResponse{}, errors.Annotate(err, "r.runner.RunCommands")
	}
	return *runResult, nil
}

// NewRunListener returns a new RunListener that is listening on given
// socket or named pipe passed in. If a valid RunListener is returned, is
// has the go routine running, and should be closed by the creator
// when they are done with it.
func NewRunListener(runner CommandRunner, socketPath string) (*RunListener, error) {
	listener, err := sockets.Listen(socketPath)
	if err != nil {
		return nil, errors.Trace(err)
	}
	runListener := &RunListener{
		listener: listener,
		srvRoot:  &jujuRunRpcRoot{runner},
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
		go func(netConn net.Conn) {
			defer s.wg.Done()
			codec := jsoncodec.NewNet(netConn)
			conn := rpc.NewConn(codec, nil)
			conn.Serve(s.srvRoot, nil)
			conn.Start()
			select {
			case <-s.closing:
				conn.Close()
			case <-conn.Dead():
			}
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
