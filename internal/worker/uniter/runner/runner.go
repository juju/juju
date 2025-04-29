// Copyright 2012-2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package runner

import (
	"bytes"
	stdcontext "context"
	"encoding/base64"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"
	"unicode/utf8"

	"github.com/juju/clock"
	"github.com/juju/errors"
	utilexec "github.com/juju/utils/v4/exec"

	"github.com/juju/juju/core/actions"
	corelogger "github.com/juju/juju/core/logger"
	"github.com/juju/juju/internal/cmd"
	"github.com/juju/juju/internal/worker/common/charmrunner"
	"github.com/juju/juju/internal/worker/uniter/runner/context"
	"github.com/juju/juju/internal/worker/uniter/runner/debug"
	"github.com/juju/juju/internal/worker/uniter/runner/jujuc"
)

// Logger is here to stop the desire of creating a package level Logger.
// Don't do this, instead use the method defined in the Runner.
type logger interface{}

var _ logger = struct{}{}

// HookHandlerType is used to indicate the type of script used for handling a
// particular hook type.
type HookHandlerType string

// String implements fmt.Stringer for HookHandlerType.
func (t HookHandlerType) String() string {
	switch t {
	case ExplicitHookHandler:
		return "explicit, bespoke hook script"
	case DispatchingHookHandler:
		return "hook dispatching script: " + hookDispatcherScript
	default:
		return "unknown/invalid hook handler"
	}
}

const (
	InvalidHookHandler = HookHandlerType("invalid")

	// ExplicitHookHandler indicates that a bespoke, per-hook script was
	// used for handling a particular hook.
	ExplicitHookHandler = HookHandlerType("explicit")

	// DispatchingHookHandler indicates the use of a specialized script that
	// acts as a dispatcher for all types of hooks. This functionality has
	// been introduced with the operator framework changes.
	DispatchingHookHandler = HookHandlerType("dispatch")

	hookDispatcherScript = "dispatch"
)

// Runner is responsible for invoking commands in a context.
type Runner interface {
	// Context returns the context against which the runner executes.
	Context() context.Context

	// RunHook executes the hook with the supplied name and returns back
	// the type of script handling hook that was used or whether any errors
	// occurred.
	RunHook(ctx stdcontext.Context, name string) (HookHandlerType, error)

	// RunAction executes the action with the supplied name.
	RunAction(ctx stdcontext.Context, name string) (HookHandlerType, error)

	// RunCommands executes the supplied script.
	RunCommands(ctx stdcontext.Context, commands string) (*utilexec.ExecResponse, error)
}

// NewRunnerFunc returns a func used to create a Runner backed by the supplied context and paths.
type NewRunnerFunc func(context context.Context, paths context.Paths, options ...Option) Runner

// Option is a functional option for NewRunner.
type Option func(*options)

type options struct {
	executor ExecFunc
}

// WithExecutor passes a custom executor to the runner.
func WithExecutor(executor ExecFunc) Option {
	return func(o *options) {
		o.executor = executor
	}
}

func newOptions() *options {
	return &options{
		executor: execOnMachine,
	}
}

// NewRunner returns a Runner backed by the supplied context and paths.
func NewRunner(context context.Context, paths context.Paths, options ...Option) Runner {
	opts := newOptions()
	for _, option := range options {
		option(opts)
	}
	return &runner{
		context:  context,
		paths:    paths,
		executor: opts.executor,
	}
}

// ExecParams holds all the necessary parameters for ExecFunc.
type ExecParams struct {
	Commands      []string
	Env           []string
	WorkingDir    string
	Clock         clock.Clock
	ProcessSetter func(context.HookProcess)
	Cancel        <-chan struct{}

	Stdout       io.ReadWriter
	StdoutLogger charmrunner.Stopper

	Stderr       io.ReadWriter
	StderrLogger charmrunner.Stopper
}

// execOnMachine executes commands on current machine.
func execOnMachine(params ExecParams) (*utilexec.ExecResponse, error) {
	command := utilexec.RunParams{
		Commands:    strings.Join(params.Commands, " "),
		WorkingDir:  params.WorkingDir,
		Environment: params.Env,
		Clock:       params.Clock,
	}
	err := command.Run()
	if err != nil {
		return nil, err
	}
	// TODO: refactor kill process and implement kill for caas exec.
	params.ProcessSetter(hookProcess{command.Process()})
	// Block and wait for process to finish
	return command.WaitWithCancel(params.Cancel)
}

// ExecFunc is the exec func type.
type ExecFunc func(ExecParams) (*utilexec.ExecResponse, error)

// runner implements Runner.
type runner struct {
	context context.Context
	paths   context.Paths
	// executor executes commands on a remote workload pod for CAAS.
	executor ExecFunc
}

func (runner *runner) logger() corelogger.Logger {
	return runner.context.GetLoggerByName("juju.worker.uniter.runner")
}

func (runner *runner) Context() context.Context {
	return runner.context
}

// RunCommands exists to satisfy the Runner interface.
func (runner *runner) RunCommands(ctx stdcontext.Context, commands string) (*utilexec.ExecResponse, error) {
	result, err := runner.runCommandsWithTimeout(ctx, commands, 0, clock.WallClock)
	return result, runner.context.Flush(ctx, "run commands", err)
}

// runCommandsWithTimeout is a helper to abstract common code between run commands and
// juju-exec as an action
func (runner *runner) runCommandsWithTimeout(ctx stdcontext.Context, commands string, timeout time.Duration, clock clock.Clock) (*utilexec.ExecResponse, error) {
	srv, err := runner.startJujucServer(ctx)
	if err != nil {
		return nil, err
	}
	defer srv.Close()

	environmenter := context.NewHostEnvironmenter()
	env, err := runner.context.HookVars(ctx, runner.paths, environmenter)
	if err != nil {
		return nil, errors.Trace(err)
	}

	var cancel chan struct{}
	if timeout != 0 {
		cancel = make(chan struct{})
		go func() {
			<-clock.After(timeout)
			close(cancel)
		}()
	}

	var stdout, stderr bytes.Buffer
	return runner.executor(ExecParams{
		Commands:      []string{commands},
		Env:           env,
		WorkingDir:    runner.paths.GetCharmDir(),
		Clock:         clock,
		ProcessSetter: runner.context.SetProcess,
		Cancel:        cancel,
		Stdout:        &stdout,
		Stderr:        &stderr,
	})
}

// runJujuExecAction is the function that executes when a juju-exec action is ran.
func (runner *runner) runJujuExecAction(ctx stdcontext.Context) (err error) {
	logger := runner.logger()
	logger.Debugf(ctx, "juju-exec action is running")
	data, err := runner.context.ActionData()
	if err != nil {
		return errors.Trace(err)
	}
	params := data.Params
	command, ok := params["command"].(string)
	if !ok {
		return errors.New("no command parameter to juju-exec action")
	}

	// The timeout is passed in in nanoseconds(which are represented in go as int64)
	// But due to serialization it comes out as float64
	timeout, ok := params["timeout"].(float64)
	if !ok {
		logger.Debugf(ctx, "unable to read juju-exec action timeout, will continue running action without one")
	}

	ctx = scopedActionCancel(ctx, data.Cancel)
	results, err := runner.runCommandsWithTimeout(ctx, command, time.Duration(timeout), clock.WallClock)
	if results != nil {
		if err := runner.updateActionResults(results); err != nil {
			return runner.context.Flush(ctx, "juju-exec", err)
		}
	}
	return runner.context.Flush(ctx, "juju-exec", err)
}

// scopedActionCancel returns a context that is cancelled when either the
// supplied context is cancelled or the supplied abort channel is closed.
// This is only required until actions are refactored to use a context.
func scopedActionCancel(ctx stdcontext.Context, abort <-chan struct{}) stdcontext.Context {
	c, cancel := stdcontext.WithCancel(ctx)
	go func() {
		defer cancel()

		select {
		case <-ctx.Done():
		case <-abort:
		}
	}()
	return c
}

func encodeBytes(input []byte) (value string, encoding string) {
	if utf8.Valid(input) {
		value = string(input)
		encoding = "utf8"
	} else {
		value = base64.StdEncoding.EncodeToString(input)
		encoding = "base64"
	}
	return value, encoding
}

func (runner *runner) updateActionResults(results *utilexec.ExecResponse) error {
	if err := runner.context.UpdateActionResults([]string{"return-code"}, results.Code); err != nil {
		return errors.Trace(err)
	}

	stdout, encoding := encodeBytes(results.Stdout)
	if stdout != "" {
		if err := runner.context.UpdateActionResults([]string{"stdout"}, stdout); err != nil {
			return errors.Trace(err)
		}
	}
	if encoding != "utf8" {
		if err := runner.context.UpdateActionResults([]string{"stdout-encoding"}, encoding); err != nil {
			return errors.Trace(err)
		}
	}

	stderr, encoding := encodeBytes(results.Stderr)
	if stderr != "" {
		if err := runner.context.UpdateActionResults([]string{"stderr"}, stderr); err != nil {
			return errors.Trace(err)
		}
	}
	if encoding != "utf8" {
		if err := runner.context.UpdateActionResults([]string{"stderr-encoding"}, encoding); err != nil {
			return errors.Trace(err)
		}
	}
	return nil
}

// RunAction exists to satisfy the Runner interface.
func (runner *runner) RunAction(ctx stdcontext.Context, actionName string) (HookHandlerType, error) {
	if actions.IsJujuExecAction(actionName) {
		return InvalidHookHandler, runner.runJujuExecAction(ctx)
	}
	runner.logger().Debugf(ctx, "running action %q", actionName)
	return runner.runCharmHookWithLocation(ctx, actionName, "actions")
}

// RunHook exists to satisfy the Runner interface.
func (runner *runner) RunHook(ctx stdcontext.Context, hookName string) (HookHandlerType, error) {
	return runner.runCharmHookWithLocation(ctx, hookName, "hooks")
}

func (runner *runner) runCharmHookWithLocation(ctx stdcontext.Context, hookName, charmLocation string) (hookHandlerType HookHandlerType, err error) {
	srv, err := runner.startJujucServer(ctx)
	if err != nil {
		return InvalidHookHandler, errors.Trace(err)
	}
	defer srv.Close()

	environmenter := context.NewHostEnvironmenter()
	env, err := runner.context.HookVars(ctx, runner.paths, environmenter)
	if err != nil {
		return InvalidHookHandler, errors.Trace(err)
	}
	env = append(env, "JUJU_DISPATCH_PATH="+charmLocation+"/"+hookName)

	defer func() {
		err = runner.context.Flush(ctx, hookName, err)
	}()

	logger := runner.logger()
	debugctx := debug.NewHooksContext(runner.context.UnitName())
	if session, _ := debugctx.FindSession(); session != nil && session.MatchHook(hookName) {
		// Note: hookScript might be relative but the debug session only requires its name
		hookHandlerType, hookScript, err := runner.discoverHookHandler(
			hookName, runner.paths.GetCharmDir(), charmLocation)
		if session.DebugAt() != "" {
			if hookHandlerType == InvalidHookHandler {
				logger.Infof(ctx, "debug-code active, but hook %s not implemented (skipping)", hookName)
				return InvalidHookHandler, err
			}
			logger.Infof(ctx, "executing %s via debug-code; %s", hookName, hookHandlerType)
		} else {
			logger.Infof(ctx, "executing %s via debug-hooks; %s", hookName, hookHandlerType)
		}
		return hookHandlerType, session.RunHook(hookName, runner.paths.GetCharmDir(), env, hookScript)
	}

	charmDir := runner.paths.GetCharmDir()
	hookHandlerType, hookScript, err := runner.discoverHookHandler(hookName, charmDir, charmLocation)
	if err != nil {
		return InvalidHookHandler, err
	}
	return hookHandlerType, runner.runCharmProcessOnLocal(hookScript, hookName, charmDir, env)
}

// loggerAdaptor implements MessageReceiver and
// sends messages to a logger.
type loggerAdaptor struct {
	corelogger.Logger
	level corelogger.Level
}

// Messagef implements the charmrunner MessageReceiver interface
func (l *loggerAdaptor) Messagef(isPrefix bool, message string, args ...interface{}) {
	l.Logf(stdcontext.Background(), l.level, corelogger.Labels{}, message, args...)
}

// bufferAdaptor implements MessageReceiver and
// is used with the out writer from os.Pipe().
// It allows the hook logger to grab console output
// as well as passing the output to an action result.
type bufferAdaptor struct {
	io.ReadWriter

	mu      sync.Mutex
	outCopy bytes.Buffer
}

// Read implements the io.Reader interface
func (b *bufferAdaptor) Read(p []byte) (n int, err error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.outCopy.Read(p)
}

// Messagef implements the charmrunner MessageReceiver interface
func (b *bufferAdaptor) Messagef(isPrefix bool, message string, args ...interface{}) {
	formattedMessage := message
	if len(args) > 0 {
		formattedMessage = fmt.Sprintf(message, args...)
	}
	if !isPrefix {
		formattedMessage += "\n"
	}

	b.mu.Lock()
	defer b.mu.Unlock()
	b.outCopy.WriteString(formattedMessage)
}

// Bytes exposes the underlying buffered bytes.
func (b *bufferAdaptor) Bytes() []byte {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.outCopy.Bytes()
}

const (
	// ErrTerminated indicate the hook or action exited due to a SIGTERM or SIGKILL signal.
	ErrTerminated = errors.ConstError("terminated")
)

// Check still tested
func (runner *runner) runCharmProcessOnLocal(hook, hookName, charmDir string, env []string) error {
	ps := exec.Command(hook)
	ps.Env = env
	ps.Dir = charmDir
	outReader, outWriter, err := os.Pipe()
	if err != nil {
		return errors.Errorf("cannot make logging pipe: %v", err)
	}
	defer func() { _ = outWriter.Close() }()

	ps.Stdout = outWriter
	hookOutLogger := charmrunner.NewHookLogger(outReader,
		&loggerAdaptor{Logger: runner.getLogger(hookName), level: corelogger.DEBUG},
	)
	go hookOutLogger.Run()
	defer hookOutLogger.Stop()

	errReader, errWriter, err := os.Pipe()
	if err != nil {
		return errors.Errorf("cannot make stderr logging pipe: %v", err)
	}
	defer func() { _ = errWriter.Close() }()

	ps.Stderr = errWriter
	hookErrLogger := charmrunner.NewHookLogger(errReader,
		&loggerAdaptor{Logger: runner.getLogger(hookName), level: corelogger.WARNING},
	)
	defer hookErrLogger.Stop()
	go hookErrLogger.Run()

	var cancel <-chan struct{}
	var actionOut *bufferAdaptor
	var actionErr *bufferAdaptor
	actionData, err := runner.context.ActionData()
	runningAction := err == nil && actionData != nil
	if runningAction {
		actionOut = &bufferAdaptor{ReadWriter: outWriter}
		hookOutLogger.AddReceiver(actionOut)
		actionErr = &bufferAdaptor{ReadWriter: errWriter}
		hookErrLogger.AddReceiver(actionErr)
		cancel = actionData.Cancel
	}

	err = ps.Start()
	var exitErr error
	if err == nil {
		done := make(chan struct{})
		if cancel != nil {
			go func() {
				select {
				case <-cancel:
					_ = ps.Process.Kill()
				case <-done:
				}
			}()
		}
		// Record the *os.Process of the hook
		runner.context.SetProcess(hookProcess{ps.Process})
		// Block until execution finishes
		exitErr = ps.Wait()
		close(done)
	} else {
		exitErr = err
	}

	// Ensure hook loggers are stopped before reading stdout/stderr
	// so all the output is captured.
	hookOutLogger.Stop()
	hookErrLogger.Stop()

	// If we are running an action, record stdout and stderr.
	if runningAction {
		resp := &utilexec.ExecResponse{
			Code:   ps.ProcessState.ExitCode(),
			Stdout: actionOut.Bytes(),
			Stderr: actionErr.Bytes(),
		}
		if err := runner.updateActionResults(resp); err != nil {
			return errors.Trace(err)
		}
	}
	if exitError, ok := exitErr.(*exec.ExitError); ok && exitError != nil {
		waitStatus := exitError.ProcessState.Sys().(syscall.WaitStatus)
		if waitStatus.Signal() == syscall.SIGTERM || waitStatus.Signal() == syscall.SIGKILL {
			return errors.Trace(ErrTerminated)
		}
	}

	return errors.Trace(exitErr)
}

// discoverHookHandler checks to see if the dispatch script exists, if not,
// check for the given hookName.  Based on what is discovered, return the
// HookHandlerType and the actual script to be run.
func (runner *runner) discoverHookHandler(hookName, charmDir, charmLocation string) (HookHandlerType, string, error) {
	err := checkCharmExists(charmDir)
	if err != nil {
		return InvalidHookHandler, "", errors.Trace(err)
	}
	hook, err := discoverHookScript(charmDir, hookDispatcherScript)
	if err == nil {
		return DispatchingHookHandler, hook, nil
	}
	if !charmrunner.IsMissingHookError(err) {
		return InvalidHookHandler, "", err
	}
	if hook, err = discoverHookScript(charmDir, filepath.Join(charmLocation, hookName)); err == nil {
		return ExplicitHookHandler, hook, nil
	}
	return InvalidHookHandler, hook, err
}

func (runner *runner) startJujucServer(ctx stdcontext.Context) (*jujuc.Server, error) {
	// Prepare server.
	getCmd := func(ctxId, cmdName string) (cmd.Command, error) {
		if ctxId != runner.context.Id() {
			return nil, errors.Errorf("wrong context ID; got %q", ctxId)
		}
		return jujuc.NewCommand(runner.context, cmdName)
	}

	socket := runner.paths.GetJujucServerSocket()
	runner.logger().Debugf(ctx, "starting jujuc server %v", socket)
	srv, err := jujuc.NewServer(getCmd, socket)
	if err != nil {
		return nil, errors.Annotate(err, "starting jujuc server")
	}
	go func() { _ = srv.Run() }()
	return srv, nil
}

// getLogger returns the logger for a particular unit's hook.
func (runner *runner) getLogger(hookName string) corelogger.Logger {
	return runner.context.GetLoggerByName(fmt.Sprintf("unit.%s.%s", runner.context.UnitName(), hookName))
}

type hookProcess struct {
	*os.Process
}

func (p hookProcess) Pid() int {
	return p.Process.Pid
}
