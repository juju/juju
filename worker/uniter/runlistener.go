// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// The run listener is a worker go-routine that listens on either a unix
// socket or a tcp connection for juju-run commands.

package uniter

import (
	"net"
	"net/rpc"
	"sync"

	"launchpad.net/tomb"

	"github.com/juju/errors"
	"github.com/juju/utils/exec"

	"github.com/juju/juju/juju/sockets"
	"github.com/juju/juju/worker"
	"github.com/juju/juju/worker/uniter/operation"
	"github.com/juju/juju/worker/uniter/runcommands"
)

const JujuRunEndpoint = "JujuRunServer.RunCommands"

var errCommandAborted = errors.New("command execution aborted")

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

// RunListenerConfig contains the configuration for a RunListener.
type RunListenerConfig struct {
	// SocketPath is the path of the socket to listen on for run commands.
	SocketPath string

	// CommandRunner is the CommandRunner that will run commands.
	CommandRunner CommandRunner
}

func (cfg *RunListenerConfig) Validate() error {
	if cfg.SocketPath == "" {
		return errors.NotValidf("SocketPath unspecified")
	}
	if cfg.CommandRunner == nil {
		return errors.NotValidf("CommandRunner unspecified")
	}
	return nil
}

// RunListener is responsible for listening on the network connection and
// setting up the rpc server on that net connection. Also starts the go routine
// that listens and hands off the work.
type RunListener struct {
	RunListenerConfig
	listener net.Listener
	server   *rpc.Server
	closed   chan struct{}
	closing  chan struct{}
	wg       sync.WaitGroup
}

// NewRunListener returns a new RunListener that is listening on given
// socket or named pipe passed in. If a valid RunListener is returned, is
// has the go routine running, and should be closed by the creator
// when they are done with it.
func NewRunListener(cfg RunListenerConfig) (*RunListener, error) {
	if err := cfg.Validate(); err != nil {
		return nil, errors.Trace(err)
	}
	listener, err := sockets.Listen(cfg.SocketPath)
	if err != nil {
		return nil, errors.Trace(err)
	}
	runListener := &RunListener{
		RunListenerConfig: cfg,
		listener:          listener,
		server:            rpc.NewServer(),
		closed:            make(chan struct{}),
		closing:           make(chan struct{}),
	}
	if err := runListener.server.Register(&JujuRunServer{runListener}); err != nil {
		return nil, errors.Trace(err)
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
func (s *RunListener) Close() error {
	defer func() {
		<-s.closed
		logger.Debugf("juju-run listener stopped")
	}()
	close(s.closing)
	return s.listener.Close()
}

// RunCommands executes the supplied commands in a hook context.
func (r *RunListener) RunCommands(args RunCommandsArgs) (results *exec.ExecResponse, err error) {
	logger.Tracef("run commands: %s", args.Commands)
	return r.CommandRunner.RunCommands(args)
}

// newRunListenerWrapper returns a worker that will Close the supplied run
// listener when the worker is killed. The Wait() method will never return
// an error -- NewRunListener just drops the Run error on the floor and that's
// not what I'm fixing here.
func newRunListenerWrapper(rl *RunListener) worker.Worker {
	rlw := &runListenerWrapper{rl: rl}
	go func() {
		defer rlw.tomb.Done()
		defer rlw.tearDown()
		<-rlw.tomb.Dying()
	}()
	return rlw
}

type runListenerWrapper struct {
	tomb tomb.Tomb
	rl   *RunListener
}

func (rlw *runListenerWrapper) tearDown() {
	if err := rlw.rl.Close(); err != nil {
		logger.Warningf("error closing runlistener: %v", err)
	}
}

// Kill is part of the worker.Worker interface.
func (rlw *runListenerWrapper) Kill() {
	rlw.tomb.Kill(nil)
}

// Wait is part of the worker.Worker interface.
func (rlw *runListenerWrapper) Wait() error {
	return rlw.tomb.Wait()
}

// The JujuRunServer is the entity that has the methods that are called over
// the rpc connection.
type JujuRunServer struct {
	runner CommandRunner
}

// RunCommands delegates the actual running to the runner and populates the
// response structure.
func (r *JujuRunServer) RunCommands(args RunCommandsArgs, result *exec.ExecResponse) error {
	logger.Debugf("RunCommands: %+v", args)
	runResult, err := r.runner.RunCommands(args)
	if err != nil {
		return errors.Annotate(err, "r.runner.RunCommands")
	}
	*result = *runResult
	return err
}

// ChannelCommandRunnerConfig contains the configuration for a ChannelCommandRunner.
type ChannelCommandRunnerConfig struct {
	// Abort is a channel that will be closed when the runner should abort
	// the execution of run commands.
	Abort <-chan struct{}

	// Commands is used to add commands received from the listener.
	Commands runcommands.Commands

	// CommandChannel will be sent the IDs of commands added to Commands.
	CommandChannel chan<- string
}

func (cfg ChannelCommandRunnerConfig) Validate() error {
	if cfg.Abort == nil {
		return errors.NotValidf("Abort unspecified")
	}
	if cfg.Commands == nil {
		return errors.NotValidf("Commands unspecified")
	}
	if cfg.CommandChannel == nil {
		return errors.NotValidf("CommandChannel unspecified")
	}
	return nil
}

// ChannelCommandRunner is a CommandRunner that registers command
// arguments in a runcommands.Commands, sends the returned IDs to
// a channel and waits for response callbacks.
type ChannelCommandRunner struct {
	config ChannelCommandRunnerConfig
}

// NewChannelCommandRunner returns a new ChannelCommandRunner with the
// given configuration.
func NewChannelCommandRunner(cfg ChannelCommandRunnerConfig) (*ChannelCommandRunner, error) {
	if err := cfg.Validate(); err != nil {
		return nil, errors.Trace(err)
	}
	return &ChannelCommandRunner{cfg}, nil
}

// RunCommands executes the supplied run commands by registering the
// arguments in a runcommands.Commands, and then sending the returned
// ID to a channel and waiting for a response callback.
func (c *ChannelCommandRunner) RunCommands(args RunCommandsArgs) (results *exec.ExecResponse, err error) {
	type responseInfo struct {
		response *exec.ExecResponse
		err      error
	}

	// NOTE(axw) the response channel must be synchronous so that the
	// response is received before the uniter resumes operation, and
	// potentially aborts. This prevents a race when rebooting.
	responseChan := make(chan responseInfo)
	responseFunc := func(response *exec.ExecResponse, err error) {
		select {
		case <-c.config.Abort:
		case responseChan <- responseInfo{response, err}:
		}
	}

	id := c.config.Commands.AddCommand(
		operation.CommandArgs{
			Commands:        args.Commands,
			RelationId:      args.RelationId,
			RemoteUnitName:  args.RemoteUnitName,
			ForceRemoteUnit: args.ForceRemoteUnit,
		},
		responseFunc,
	)
	select {
	case <-c.config.Abort:
		return nil, errCommandAborted
	case c.config.CommandChannel <- id:
	}

	select {
	case <-c.config.Abort:
		return nil, errCommandAborted
	case response := <-responseChan:
		return response.response, response.err
	}
}
