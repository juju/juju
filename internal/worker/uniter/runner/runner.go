// Copyright 2012-2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package runner

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"syscall"
	"time"
	"unicode/utf8"

	"github.com/juju/clock"
	"github.com/juju/cmd/v3"
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/utils/v3"
	utilexec "github.com/juju/utils/v3/exec"
	"github.com/kballard/go-shellquote"

	"github.com/juju/juju/core/actions"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/internal/worker/common/charmrunner"
	"github.com/juju/juju/internal/worker/uniter/runner/context"
	"github.com/juju/juju/internal/worker/uniter/runner/debug"
	"github.com/juju/juju/internal/worker/uniter/runner/jujuc"
)

// Logger is here to stop the desire of creating a package level Logger.
// Don't do this, instead use the method defined in the Runner.
type logger interface{}

var _ logger = struct{}{}

type runMode int

const (
	runOnUnknown runMode = iota
	runOnLocal
	runOnRemote
)

// RunLocation dictates where to execute commands.
type RunLocation string

const (
	// Operator runs where the operator/uniter is running.
	Operator = RunLocation("operator")
	// Workload runs where the workload is running.
	Workload = RunLocation("workload")
)

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
	RunHook(name string) (HookHandlerType, error)

	// RunAction executes the action with the supplied name.
	RunAction(name string) (HookHandlerType, error)

	// RunCommands executes the supplied script.
	RunCommands(commands string, runLocation RunLocation) (*utilexec.ExecResponse, error)
}

// NewRunnerFunc returns a func used to create a Runner backed by the supplied context and paths.
type NewRunnerFunc func(context context.Context, paths context.Paths, remoteExecutor ExecFunc, options ...Option) Runner

// Option is a functional option for NewRunner.
type Option func(*options)

type options struct {
	tokenGenerator TokenGenerator
}

// WithTokenGenerator returns an Option that sets the token generator for the
// runner.
func WithTokenGenerator(tg TokenGenerator) Option {
	return func(o *options) {
		o.tokenGenerator = tg
	}
}

func newOptions() *options {
	return &options{
		tokenGenerator: &tokenGenerator{},
	}
}

// NewRunner returns a Runner backed by the supplied context and paths.
func NewRunner(context context.Context, paths context.Paths, remoteExecutor ExecFunc, options ...Option) Runner {
	opts := newOptions()
	for _, option := range options {
		option(opts)
	}
	return &runner{
		context:        context,
		paths:          paths,
		remoteExecutor: remoteExecutor,
		tokenGenerator: opts.tokenGenerator,
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

// TokenGenerator is the interface for generating tokens.
type TokenGenerator interface {
	// Generate generates a token based on the remote flag.
	// If remote is false, it returns an empty string. Otherwise, it returns a
	// random token.
	Generate(remote bool) (string, error)
}

// runner implements Runner.
type runner struct {
	context context.Context
	paths   context.Paths
	// remoteExecutor executes commands on a remote workload pod for CAAS.
	remoteExecutor ExecFunc
	tokenGenerator TokenGenerator
}

func (runner *runner) logger() loggo.Logger {
	return runner.context.GetLogger("juju.worker.uniter.runner")
}

func (runner *runner) Context() context.Context {
	return runner.context
}

func (runner *runner) getExecutor(rMode runMode) (ExecFunc, error) {
	switch rMode {
	case runOnLocal:
		return execOnMachine, nil
	case runOnRemote:
		if runner.remoteExecutor != nil {
			return runner.remoteExecutor, nil
		}
	}
	return nil, errors.NotSupportedf("run command mode %q", rMode)
}

func (runner *runner) runLocationToMode(runLocation RunLocation) (runMode, error) {
	switch runLocation {
	case Operator:
		return runOnLocal, nil
	case Workload:
		if runner.context.ModelType() == model.CAAS && runner.remoteExecutor != nil {
			return runOnRemote, nil
		}
		return runOnLocal, nil
	default:
		return runOnUnknown, errors.NotValidf("RunLocation %q", runLocation)
	}
}

// RunCommands exists to satisfy the Runner interface.
func (runner *runner) RunCommands(commands string, runLocation RunLocation) (*utilexec.ExecResponse, error) {
	rMode, err := runner.runLocationToMode(runLocation)
	if err != nil {
		return nil, errors.Trace(err)
	}
	result, err := runner.runCommandsWithTimeout(commands, 0, clock.WallClock, rMode, nil)
	return result, runner.context.Flush("run commands", err)
}

// runCommandsWithTimeout is a helper to abstract common code between run commands and
// juju-exec as an action
func (runner *runner) runCommandsWithTimeout(commands string, timeout time.Duration, clock clock.Clock, rMode runMode, abort <-chan struct{}) (*utilexec.ExecResponse, error) {
	token, err := runner.tokenGenerator.Generate(rMode == runOnRemote)
	if err != nil {
		return nil, errors.Trace(err)
	}

	srv, err := runner.startJujucServer(token, rMode)
	if err != nil {
		return nil, err
	}
	defer srv.Close()

	environmenter := context.NewHostEnvironmenter()
	if rMode == runOnRemote {
		env, err := runner.getRemoteEnviron(abort)
		if err != nil {
			return nil, errors.Annotatef(err, "getting remote environ")
		}
		environmenter = context.NewRemoteEnvironmenter(
			func() []string {
				rval := make([]string, 0, len(env))
				for k, v := range env {
					rval = append(rval, fmt.Sprintf("%s=%s", k, v))
				}
				return rval
			},
			func(k string) string {
				return env[k]
			},
			func(k string) (string, bool) {
				v, t := env[k]
				return v, t
			},
		)
	}
	env, err := runner.context.HookVars(runner.paths, rMode == runOnRemote, environmenter)
	if err != nil {
		return nil, errors.Trace(err)
	}
	if rMode == runOnRemote {
		env = append(env, "JUJU_AGENT_TOKEN="+token)
	}

	var cancel chan struct{}
	if timeout != 0 {
		cancel = make(chan struct{})
		go func() {
			<-clock.After(timeout)
			close(cancel)
		}()
	}

	executor, err := runner.getExecutor(rMode)
	if err != nil {
		return nil, errors.Trace(err)
	}
	var stdout, stderr bytes.Buffer
	return executor(ExecParams{
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
func (runner *runner) runJujuExecAction() (err error) {
	logger := runner.logger()
	logger.Debugf("juju-exec action is running")
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
		logger.Debugf("unable to read juju-exec action timeout, will continue running action without one")
	}

	runLocation := Operator
	if workloadContext, _ := params["workload-context"].(bool); workloadContext {
		runLocation = Workload
	}
	rMode, err := runner.runLocationToMode(runLocation)
	if err != nil {
		return errors.Trace(err)
	}

	results, err := runner.runCommandsWithTimeout(command, time.Duration(timeout), clock.WallClock, rMode, data.Cancel)
	if results != nil {
		if err := runner.updateActionResults(results); err != nil {
			return runner.context.Flush("juju-exec", err)
		}
	}
	return runner.context.Flush("juju-exec", err)
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
func (runner *runner) RunAction(actionName string) (HookHandlerType, error) {
	data, err := runner.context.ActionData()
	if err != nil {
		return InvalidHookHandler, errors.Trace(err)
	}
	if actions.IsJujuExecAction(actionName) {
		return InvalidHookHandler, runner.runJujuExecAction()
	}
	runLocation := Operator
	if workloadContext, ok := data.Params["workload-context"].(bool); !ok || workloadContext {
		runLocation = Workload
	}
	rMode, err := runner.runLocationToMode(runLocation)
	if err != nil {
		return InvalidHookHandler, errors.Trace(err)
	}
	runner.logger().Debugf("running action %q on %v", actionName, rMode)
	return runner.runCharmHookWithLocation(actionName, "actions", rMode)
}

// RunHook exists to satisfy the Runner interface.
func (runner *runner) RunHook(hookName string) (HookHandlerType, error) {
	return runner.runCharmHookWithLocation(hookName, "hooks", runOnLocal)
}

func (runner *runner) runCharmHookWithLocation(hookName, charmLocation string, rMode runMode) (hookHandlerType HookHandlerType, err error) {
	token, err := runner.tokenGenerator.Generate(rMode == runOnRemote)
	if err != nil {
		return InvalidHookHandler, errors.Trace(err)
	}

	srv, err := runner.startJujucServer(token, rMode)
	if err != nil {
		return InvalidHookHandler, errors.Trace(err)
	}
	defer srv.Close()

	environmenter := context.NewHostEnvironmenter()
	if rMode == runOnRemote {
		var cancel <-chan struct{}
		actionData, err := runner.context.ActionData()
		if err == nil && actionData != nil {
			cancel = actionData.Cancel
		}
		env, err := runner.getRemoteEnviron(cancel)
		if err != nil {
			return InvalidHookHandler, errors.Annotatef(err, "getting remote environ")
		}
		environmenter = context.NewRemoteEnvironmenter(
			func() []string {
				rval := make([]string, 0, len(env))
				for k, v := range env {
					rval = append(rval, fmt.Sprintf("%s=%s", k, v))
				}
				return rval
			},
			func(k string) string {
				return env[k]
			},
			func(k string) (string, bool) {
				v, t := env[k]
				return v, t
			},
		)
	}

	env, err := runner.context.HookVars(runner.paths, rMode == runOnRemote, environmenter)
	if err != nil {
		return InvalidHookHandler, errors.Trace(err)
	}
	if rMode == runOnRemote {
		env = append(env, "JUJU_AGENT_TOKEN="+token)
	}
	env = append(env, "JUJU_DISPATCH_PATH="+charmLocation+"/"+hookName)

	defer func() {
		err = runner.context.Flush(hookName, err)
	}()

	logger := runner.logger()
	debugctx := debug.NewHooksContext(runner.context.UnitName())
	if session, _ := debugctx.FindSession(); session != nil && session.MatchHook(hookName) {
		// Note: hookScript might be relative but the debug session only requires its name
		hookHandlerType, hookScript, err := runner.discoverHookHandler(
			hookName, runner.paths.GetCharmDir(), charmLocation)
		if session.DebugAt() != "" {
			if hookHandlerType == InvalidHookHandler {
				logger.Infof("debug-code active, but hook %s not implemented (skipping)", hookName)
				return InvalidHookHandler, err
			}
			logger.Infof("executing %s via debug-code; %s", hookName, hookHandlerType)
		} else {
			logger.Infof("executing %s via debug-hooks; %s", hookName, hookHandlerType)
		}
		return hookHandlerType, session.RunHook(hookName, runner.paths.GetCharmDir(), env, hookScript)
	}

	charmDir := runner.paths.GetCharmDir()
	hookHandlerType, hookScript, err := runner.discoverHookHandler(hookName, charmDir, charmLocation)
	if err != nil {
		return InvalidHookHandler, err
	}
	if rMode == runOnRemote {
		return hookHandlerType, runner.runCharmProcessOnRemote(hookScript, hookName, charmDir, env)
	}
	return hookHandlerType, runner.runCharmProcessOnLocal(hookScript, hookName, charmDir, env)
}

// loggerAdaptor implements MessageReceiver and
// sends messages to a logger.
type loggerAdaptor struct {
	loggo.Logger
	level loggo.Level
}

// Messagef implements the charmrunner MessageReceiver interface
func (l *loggerAdaptor) Messagef(isPrefix bool, message string, args ...interface{}) {
	l.Logf(l.level, message, args...)
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

func (runner *runner) runCharmProcessOnRemote(hook, hookName, charmDir string, env []string) error {
	var cancel <-chan struct{}
	outReader, outWriter, err := os.Pipe()
	if err != nil {
		return errors.Errorf("cannot make stdout logging pipe: %v", err)
	}
	defer func() { _ = outWriter.Close() }()

	actionOut := &bufferAdaptor{ReadWriter: outWriter}
	hookOutLogger := charmrunner.NewHookLogger(outReader,
		&loggerAdaptor{Logger: runner.getLogger(hookName), level: loggo.DEBUG},
		actionOut,
	)
	defer hookOutLogger.Stop()
	go hookOutLogger.Run()

	// When running an action, We capture stdout and stderr
	// separately to pass back.
	var actionErr = actionOut
	var hookErrLogger *charmrunner.HookLogger
	actionData, err := runner.context.ActionData()
	runningAction := err == nil && actionData != nil
	if runningAction {
		cancel = actionData.Cancel

		errReader, errWriter, err := os.Pipe()
		if err != nil {
			return errors.Errorf("cannot make stderr logging pipe: %v", err)
		}
		defer func() { _ = errWriter.Close() }()

		actionErr = &bufferAdaptor{ReadWriter: errWriter}
		hookErrLogger = charmrunner.NewHookLogger(errReader,
			&loggerAdaptor{Logger: runner.getLogger(hookName), level: loggo.WARNING},
			actionErr,
		)
		defer hookErrLogger.Stop()
		go hookErrLogger.Run()
	}

	executor, err := runner.getExecutor(runOnRemote)
	if err != nil {
		return errors.Trace(err)
	}
	resp, err := executor(
		ExecParams{
			Commands:     []string{hook},
			Env:          env,
			WorkingDir:   charmDir,
			Cancel:       cancel,
			Stdout:       actionOut,
			StdoutLogger: hookOutLogger,
			Stderr:       actionErr,
			StderrLogger: hookErrLogger,
		},
	)

	// If we are running an action, record stdout and stderr.
	if runningAction && resp != nil {
		if err := runner.updateActionResults(resp); err != nil {
			return errors.Trace(err)
		}
	}

	return errors.Trace(err)
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
		&loggerAdaptor{Logger: runner.getLogger(hookName), level: loggo.DEBUG},
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
		&loggerAdaptor{Logger: runner.getLogger(hookName), level: loggo.WARNING},
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

func (runner *runner) startJujucServer(token string, rMode runMode) (*jujuc.Server, error) {
	// Prepare server.
	getCmd := func(ctxId, cmdName string) (cmd.Command, error) {
		if ctxId != runner.context.Id() {
			return nil, errors.Errorf("wrong context ID; got %q", ctxId)
		}
		return jujuc.NewHookCommand(runner.context, cmdName)
	}

	socket := runner.paths.GetJujucServerSocket(rMode == runOnRemote)
	runner.logger().Debugf("starting jujuc server %s %v", token, socket)
	srv, err := jujuc.NewServer(getCmd, socket, token)
	if err != nil {
		return nil, errors.Annotate(err, "starting jujuc server")
	}
	go func() { _ = srv.Run() }()
	return srv, nil
}

// getLogger returns the logger for a particular unit's hook.
func (runner *runner) getLogger(hookName string) loggo.Logger {
	return runner.context.GetLogger(fmt.Sprintf("unit.%s.%s", runner.context.UnitName(), hookName))
}

var exportLineRegexp = regexp.MustCompile("(?m)^export ([^=]+)=(.*)$")

func (runner *runner) getRemoteEnviron(abort <-chan struct{}) (map[string]string, error) {
	remoteExecutor, err := runner.getExecutor(runOnRemote)
	if err != nil {
		return nil, errors.Trace(err)
	}

	var stdout, stderr bytes.Buffer
	res, err := remoteExecutor(ExecParams{
		Commands: []string{"unset _; export"},
		Cancel:   abort,
		Stdout:   &stdout,
		Stderr:   &stderr,
	})
	if err != nil {
		if res != nil {
			err = errors.Annotatef(err, "stdout: %q stderr: %q", string(res.Stdout), string(res.Stderr))
		}
		return nil, errors.Trace(err)
	}
	matches := exportLineRegexp.FindAllStringSubmatch(string(res.Stdout), -1)
	env := map[string]string{}
	for _, values := range matches {
		if len(values) != 3 {
			return nil, errors.Errorf("regex returned incorrect submatch count")
		}
		key := values[1]
		value := values[2]
		unquoted, err := shellquote.Split(value)
		if err != nil {
			return nil, errors.Annotatef(err, "failed to unquote %s", value)
		}
		if len(unquoted) != 1 {
			return nil, errors.Errorf("shellquote returned too many strings")
		}
		unquotedValue := unquoted[0]
		env[key] = unquotedValue
	}
	runner.logger().Debugf("fetched remote env %+q", env)
	return env, nil
}

type hookProcess struct {
	*os.Process
}

func (p hookProcess) Pid() int {
	return p.Process.Pid
}

type tokenGenerator struct{}

// Generate generates a token based on the remote flag.
// If remote is false, it returns an empty string. Otherwise, it returns a
// random token.
func (t *tokenGenerator) Generate(remote bool) (string, error) {
	if !remote {
		return "", nil
	}
	token, err := utils.RandomPassword()
	if err != nil {
		return "", errors.Trace(err)
	}
	return token, nil
}
