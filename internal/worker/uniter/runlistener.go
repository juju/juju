// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// The run listener is a worker go-routine that listens on either a unix
// socket or a tcp connection for juju-exec commands.

package uniter

import (
	stdcontext "context"
	"net"
	"net/rpc"
	"sync"

	"github.com/juju/errors"
	"github.com/juju/utils/v4/exec"
	"github.com/juju/worker/v4"
	"gopkg.in/tomb.v2"

	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/internal/worker/uniter/operation"
	"github.com/juju/juju/internal/worker/uniter/runcommands"
	"github.com/juju/juju/juju/sockets"
)

const JujuExecEndpoint = "JujuExecServer.RunCommands"

var errCommandAborted = errors.New("command execution aborted")

// RunCommandsArgs stores the arguments for a RunCommands call.
type RunCommandsArgs struct {
	// Commands is the arbitrary commands to execute on the unit
	Commands string
	// RelationId is the relation context to execute the commands in.
	RelationId int
	// RemoteUnitName is the remote unit for the relation context.
	RemoteUnitName string
	// RemoteUnitName is the remote unit for the relation context.
	RemoteApplicationName string
	// ForceRemoteUnit skips relation membership and existence validation.
	ForceRemoteUnit bool
	// UnitName is the unit for which the command is being run.
	UnitName string
}

// A CommandRunner is something that will actually execute the commands and
// return the results of that execution in the exec.ExecResponse (which
// contains stdout, stderr, and return code).
type CommandRunner interface {
	RunCommands(RunCommandsArgs RunCommandsArgs) (results *exec.ExecResponse, err error)
}

// RunListener is responsible for listening on the network connection and
// setting up the rpc server on that net connection. Also starts the go routine
// that listens and hands off the work.
type RunListener struct {
	logger logger.Logger

	mu sync.Mutex

	// commandRunners holds the CommandRunner that will run commands
	// for each unit name.
	commandRunners map[string]CommandRunner

	listener net.Listener
	server   *rpc.Server
	closed   chan struct{}
	closing  chan struct{}
	wg       sync.WaitGroup

	requiresAuth bool
}

// NewRunListener returns a new RunListener that is listening on given
// socket or named pipe passed in. If a valid RunListener is returned, is
// has the go routine running, and should be closed by the creator
// when they are done with it.
func NewRunListener(socket sockets.Socket, logger logger.Logger) (*RunListener, error) {
	listener, err := sockets.Listen(socket)
	if err != nil {
		return nil, errors.Trace(err)
	}
	runListener := &RunListener{
		logger:         logger,
		listener:       listener,
		commandRunners: make(map[string]CommandRunner),
		server:         rpc.NewServer(),
		closed:         make(chan struct{}),
		closing:        make(chan struct{}),
	}
	if socket.Network == "tcp" || socket.TLSConfig != nil {
		runListener.requiresAuth = true
	}
	if err := runListener.server.Register(&JujuExecServer{runListener, logger}); err != nil {
		return nil, errors.Trace(err)
	}
	// TODO (stickupkid) - We should probably log out when an accept fails, so
	// we can at least track it.
	go func() { _ = runListener.Run() }()
	return runListener, nil
}

// Run accepts new connections until it encounters an error, or until Close is
// called, and then blocks until all existing connections have been closed.
func (r *RunListener) Run() (err error) {
	r.logger.Debugf(stdcontext.TODO(), "juju-exec listener running")
	var conn net.Conn
	for {
		conn, err = r.listener.Accept()
		if err != nil {
			break
		}
		r.wg.Add(1)
		go func(conn net.Conn) {
			r.server.ServeConn(conn)
			r.wg.Done()
		}(conn)
	}
	r.logger.Debugf(stdcontext.TODO(), "juju-exec listener stopping")
	select {
	case <-r.closing:
		// Someone has called Close(), so it is overwhelmingly likely that
		// the error from Accept is a direct result of the Listener being
		// closed, and can therefore be safely ignored.
		err = nil
	default:
	}
	r.wg.Wait()
	close(r.closed)
	return
}

// Close immediately stops accepting connections, and blocks until all existing
// connections have been closed.
func (r *RunListener) Close() error {
	defer func() {
		<-r.closed
		r.logger.Debugf(stdcontext.TODO(), "juju-exec listener stopped")
	}()
	close(r.closing)
	return r.listener.Close()
}

// RegisterRunner registers a command runner for a given unit.
func (r *RunListener) RegisterRunner(unitName string, runner CommandRunner) {
	r.mu.Lock()
	r.commandRunners[unitName] = runner
	r.mu.Unlock()
}

// UnregisterRunner unregisters a command runner for a given unit.
func (r *RunListener) UnregisterRunner(unitName string) {
	r.mu.Lock()
	delete(r.commandRunners, unitName)
	r.mu.Unlock()
}

// RunCommands executes the supplied commands in a hook context.
func (r *RunListener) RunCommands(args RunCommandsArgs) (results *exec.ExecResponse, err error) {
	r.logger.Debugf(stdcontext.TODO(), "run commands on unit %v: %s", args.UnitName, args.Commands)
	if args.UnitName == "" {
		return nil, errors.New("missing unit name running command")
	}
	r.mu.Lock()
	runner, ok := r.commandRunners[args.UnitName]
	r.mu.Unlock()
	if !ok {
		return nil, errors.Errorf("no runner is registered for unit %v", args.UnitName)
	}

	return runner.RunCommands(args)
}

// NewRunListenerWrapper returns a worker that will Close the supplied run
// listener when the worker is killed. The Wait() method will never return
// an error -- NewRunListener just drops the Run error on the floor and that's
// not what I'm fixing here.
func NewRunListenerWrapper(rl *RunListener, logger logger.Logger) worker.Worker {
	rlw := &runListenerWrapper{logger: logger, rl: rl}
	rlw.tomb.Go(func() error {
		defer rlw.tearDown()
		<-rlw.tomb.Dying()
		return nil
	})
	return rlw
}

type runListenerWrapper struct {
	logger logger.Logger
	tomb   tomb.Tomb
	rl     *RunListener
}

func (rlw *runListenerWrapper) tearDown() {
	if err := rlw.rl.Close(); err != nil {
		rlw.logger.Warningf(stdcontext.TODO(), "error closing runlistener: %v", err)
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

// The JujuExecServer is the entity that has the methods that are called over
// the rpc connection.
type JujuExecServer struct {
	runner CommandRunner
	logger logger.Logger
}

// RunCommands delegates the actual running to the runner and populates the
// response structure.
func (r *JujuExecServer) RunCommands(args RunCommandsArgs, result *exec.ExecResponse) error {
	r.logger.Debugf(stdcontext.TODO(), "RunCommands: %+v", args)
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

	operationArgs := operation.CommandArgs{
		Commands:       args.Commands,
		RelationId:     args.RelationId,
		RemoteUnitName: args.RemoteUnitName,
		// TODO(jam): 2019-10-24 Include RemoteAppName
		ForceRemoteUnit: args.ForceRemoteUnit,
	}
	if err := operationArgs.Validate(); err != nil {
		return nil, errors.Trace(err)
	}

	type responseInfo struct {
		response *exec.ExecResponse
		err      error
	}

	// NOTE(axw) the response channel must be synchronous so that the
	// response is received before the uniter resumes operation, and
	// potentially aborts. This prevents a race when rebooting.
	responseChan := make(chan responseInfo)
	responseFunc := func(response *exec.ExecResponse, err error) bool {
		select {
		case <-c.config.Abort:
			return false
		case responseChan <- responseInfo{response, err}:
			return true
		}
	}

	id := c.config.Commands.AddCommand(operationArgs, responseFunc)
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
