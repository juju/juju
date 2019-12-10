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
	"strings"
	"sync"
	"syscall"
	"time"
	"unicode/utf8"

	"github.com/juju/clock"
	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/loggo"
	jujuos "github.com/juju/os"
	"github.com/juju/utils"
	utilexec "github.com/juju/utils/exec"

	"github.com/juju/juju/core/actions"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/worker/common/charmrunner"
	"github.com/juju/juju/worker/uniter/runner/context"
	"github.com/juju/juju/worker/uniter/runner/debug"
	"github.com/juju/juju/worker/uniter/runner/jujuc"
)

var logger = loggo.GetLogger("juju.worker.uniter.runner")

type runMode int

const (
	runOnLocal runMode = iota
	runOnRemote
)

// Runner is responsible for invoking commands in a context.
type Runner interface {

	// Context returns the context against which the runner executes.
	Context() Context

	// RunHook executes the hook with the supplied name.
	RunHook(name string) error

	// RunAction executes the action with the supplied name.
	RunAction(name string) error

	// RunCommands executes the supplied script.
	RunCommands(commands string) (*utilexec.ExecResponse, error)
}

// Context exposes hooks.Context, and additional methods needed by Runner.
type Context interface {
	jujuc.Context
	Id() string
	HookVars(paths context.Paths, remote bool) ([]string, error)
	ActionData() (*context.ActionData, error)
	SetProcess(process context.HookProcess)
	HasExecutionSetUnitStatus() bool
	ResetExecutionSetUnitStatus()
	ModelType() model.ModelType

	Prepare() error
	Flush(badge string, failure error) error
}

// NewRunner returns a Runner backed by the supplied context and paths.
func NewRunner(context Context, paths context.Paths, remoteExecutor ExecFunc) Runner {
	return &runner{context, paths, remoteExecutor}
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
	context Context
	paths   context.Paths
	// remoteExecutor executes commands on a remote workload pod for CAAS.
	remoteExecutor ExecFunc
}

func (runner *runner) Context() Context {
	return runner.context
}

func (runner *runner) getRemoteExecutor(rMode runMode) (ExecFunc, error) {
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

// RunCommands exists to satisfy the Runner interface.
func (runner *runner) RunCommands(commands string) (*utilexec.ExecResponse, error) {
	runMode := runOnLocal
	if runner.context.ModelType() == model.CAAS {
		runMode = runOnRemote
	}
	result, err := runner.runCommandsWithTimeout(commands, 0, clock.WallClock, runMode)
	return result, runner.context.Flush("run commands", err)
}

// runCommandsWithTimeout is a helper to abstract common code between run commands and
// juju-run as an action
func (runner *runner) runCommandsWithTimeout(commands string, timeout time.Duration, clock clock.Clock, rMode runMode) (*utilexec.ExecResponse, error) {
	var err error
	token := ""
	if rMode == runOnRemote {
		token, err = utils.RandomPassword()
		if err != nil {
			return nil, errors.Trace(err)
		}
	}
	srv, err := runner.startJujucServer(token, rMode)
	if err != nil {
		return nil, err
	}
	defer srv.Close()

	env, err := runner.context.HookVars(runner.paths, rMode == runOnRemote)
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

	executor, err := runner.getRemoteExecutor(rMode)
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

// runJujuRunAction is the function that executes when a juju-run action is ran.
func (runner *runner) runJujuRunAction() (err error) {
	logger.Debugf("juju-run action is running")
	params, err := runner.context.ActionParams()
	if err != nil {
		return errors.Trace(err)
	}
	command, ok := params["command"].(string)
	if !ok {
		return errors.New("no command parameter to juju-run action")
	}

	// The timeout is passed in in nanoseconds(which are represented in go as int64)
	// But due to serialization it comes out as float64
	timeout, ok := params["timeout"].(float64)
	if !ok {
		logger.Debugf("unable to read juju-run action timeout, will continue running action without one")
	}

	rMode := runOnLocal
	if runner.context.ModelType() == model.CAAS {
		if workloadContext, _ := params["workload-context"].(bool); workloadContext {
			rMode = runOnRemote
		}
	}
	results, err := runner.runCommandsWithTimeout(command, time.Duration(timeout), clock.WallClock, rMode)
	if results != nil {
		if err := runner.updateActionResults(results); err != nil {
			return runner.context.Flush("juju-run", err)
		}
	}
	return runner.context.Flush("juju-run", err)
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
	// TODO(juju3) - use lower case here
	if err := runner.context.UpdateActionResults([]string{"Code"}, fmt.Sprintf("%d", results.Code)); err != nil {
		return errors.Trace(err)
	}

	stdout, encoding := encodeBytes(results.Stdout)
	if stdout != "" {
		if err := runner.context.UpdateActionResults([]string{"Stdout"}, stdout); err != nil {
			return errors.Trace(err)
		}
	}
	if encoding != "utf8" {
		if err := runner.context.UpdateActionResults([]string{"StdoutEncoding"}, encoding); err != nil {
			return errors.Trace(err)
		}
	}

	stderr, encoding := encodeBytes(results.Stderr)
	if stderr != "" {
		if err := runner.context.UpdateActionResults([]string{"Stderr"}, stderr); err != nil {
			return errors.Trace(err)
		}
	}
	if encoding != "utf8" {
		if err := runner.context.UpdateActionResults([]string{"StderrEncoding"}, encoding); err != nil {
			return errors.Trace(err)
		}
	}
	return nil
}

// RunAction exists to satisfy the Runner interface.
func (runner *runner) RunAction(actionName string) error {
	if _, err := runner.context.ActionData(); err != nil {
		return errors.Trace(err)
	}
	if actionName == actions.JujuRunActionName {
		return runner.runJujuRunAction()
	}
	rMode := runOnLocal
	if runner.context.ModelType() == model.CAAS {
		// run actions/functions on remote workload pod if it's caas model.
		rMode = runOnRemote
	}
	return runner.runCharmHookWithLocation(actionName, runner.getFunctionDir(), rMode)
}

func (runner *runner) getFunctionDir() string {
	charmDir := runner.paths.GetCharmDir()
	dir := filepath.Join(charmDir, "functions")
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		return "functions"
	}
	return "actions"
}

// RunHook exists to satisfy the Runner interface.
func (runner *runner) RunHook(hookName string) error {
	return runner.runCharmHookWithLocation(hookName, "hooks", runOnLocal)
}

func (runner *runner) runCharmHookWithLocation(hookName, charmLocation string, rMode runMode) (err error) {
	token := ""
	if rMode == runOnRemote {
		token, err = utils.RandomPassword()
		if err != nil {
			return errors.Trace(err)
		}
	}
	srv, err := runner.startJujucServer(token, rMode)
	if err != nil {
		return err
	}
	defer srv.Close()

	env, err := runner.context.HookVars(runner.paths, rMode == runOnRemote)
	if err != nil {
		return errors.Trace(err)
	}
	if jujuos.HostOS() == jujuos.Windows {
		// TODO(fwereade): somehow consolidate with utils/exec?
		// We don't do this on the other code path, which uses exec.RunCommands,
		// because that already has handling for windows environment requirements.
		env = mergeWindowsEnvironment(env, os.Environ())
	}
	if rMode == runOnRemote {
		env = append(env, "JUJU_AGENT_TOKEN="+token)
	}

	defer func() {
		err = runner.context.Flush(hookName, err)
	}()

	debugctx := debug.NewHooksContext(runner.context.UnitName())
	if session, _ := debugctx.FindSession(); session != nil && session.MatchHook(hookName) {
		logger.Infof("executing %s via debug-hooks", hookName)
		return session.RunHook(hookName, runner.paths.GetCharmDir(), env)
	}
	if rMode == runOnRemote {
		return runner.runCharmHookOnRemote(hookName, env, charmLocation)
	}
	return runner.runCharmHookOnLocal(hookName, env, charmLocation)
}

// loggerAdaptor implements MessageReceiver and
// sends messages to a logger.
type loggerAdaptor struct {
	loggo.Logger
}

func (l *loggerAdaptor) Messagef(isPrefix bool, message string, args ...interface{}) {
	l.Debugf(message, args...)
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

func (b *bufferAdaptor) Read(p []byte) (n int, err error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.outCopy.Read(p)
}

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

func (runner *runner) runCharmHookOnRemote(hookName string, env []string, charmLocation string) error {
	charmDir := runner.paths.GetCharmDir()
	hook := filepath.Join(charmDir, filepath.Join(charmLocation, hookName))

	var cancel chan struct{}
	outReader, outWriter, err := os.Pipe()
	if err != nil {
		return errors.Errorf("cannot make stdout logging pipe: %v", err)
	}
	defer outWriter.Close()

	actionOut := &bufferAdaptor{ReadWriter: outWriter}
	hookOutLogger := charmrunner.NewHookLogger(outReader,
		&loggerAdaptor{runner.getLogger(hookName)},
		actionOut,
	)
	defer hookOutLogger.Stop()
	go hookOutLogger.Run()

	// When running an action, We capture stdout and stderr
	// separately to pass back.
	var actionErr = actionOut
	var hookErrLogger *charmrunner.HookLogger
	_, err = runner.context.ActionData()
	runningAction := err == nil
	if runningAction {
		errReader, errWriter, err := os.Pipe()
		if err != nil {
			return errors.Errorf("cannot make stderr logging pipe: %v", err)
		}
		defer errWriter.Close()

		actionErr = &bufferAdaptor{ReadWriter: errWriter}
		hookErrLogger = charmrunner.NewHookLogger(errReader,
			&loggerAdaptor{runner.getLogger(hookName)},
			actionErr,
		)
		defer hookErrLogger.Stop()
		go hookErrLogger.Run()
	}

	executor, err := runner.getRemoteExecutor(runOnRemote)
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

func (runner *runner) runCharmHookOnLocal(hookName string, env []string, charmLocation string) error {
	charmDir := runner.paths.GetCharmDir()
	hook, err := searchHook(charmDir, filepath.Join(charmLocation, hookName))
	if err != nil {
		return err
	}
	hookCmd := hookCommand(hook)
	ps := exec.Command(hookCmd[0], hookCmd[1:]...)
	ps.Env = env
	ps.Dir = charmDir
	outReader, outWriter, err := os.Pipe()
	if err != nil {
		return errors.Errorf("cannot make logging pipe: %v", err)
	}
	defer outWriter.Close()

	ps.Stdout = outWriter
	ps.Stderr = outWriter
	actionOut := &bufferAdaptor{ReadWriter: outWriter}
	hookOutLogger := charmrunner.NewHookLogger(outReader,
		&loggerAdaptor{runner.getLogger(hookName)},
		actionOut,
	)
	go hookOutLogger.Run()
	defer hookOutLogger.Stop()

	// When running an action, We capture stdout and stderr
	// separately to pass back.
	var actionErr io.Reader
	var hookErrLogger *charmrunner.HookLogger
	_, err = runner.context.ActionData()
	runningAction := err == nil
	if runningAction {
		errReader, errWriter, err := os.Pipe()
		if err != nil {
			return errors.Errorf("cannot make stderr logging pipe: %v", err)
		}
		defer errWriter.Close()

		ps.Stderr = errWriter
		errBuf := &bufferAdaptor{ReadWriter: errWriter}
		actionErr = errBuf
		hookErrLogger = charmrunner.NewHookLogger(errReader,
			&loggerAdaptor{runner.getLogger(hookName)},
			errBuf,
		)
		defer hookErrLogger.Stop()
		go hookErrLogger.Run()
	}

	err = ps.Start()
	var exitErr error
	if err == nil {
		// Record the *os.Process of the hook
		runner.context.SetProcess(hookProcess{ps.Process})
		// Block until execution finishes
		exitErr = ps.Wait()
	} else {
		exitErr = err
	}
	// Ensure hook loggers are stopped before reading stdout/stderr
	// so all the output is captured.
	hookOutLogger.Stop()
	hookErrLogger.Stop()

	// If we are running an action, record stdout and stderr.
	if runningAction {
		readBytes := func(r io.Reader) []byte {
			var o bytes.Buffer
			o.ReadFrom(r)
			return o.Bytes()
		}
		exitCode := func(exitErr error) int {
			if exitErr != nil {
				if exitErr, ok := exitErr.(*exec.ExitError); ok {
					if status, ok := exitErr.Sys().(syscall.WaitStatus); ok {
						return status.ExitStatus()
					}
				}
				return -1
			}
			return 0
		}
		resp := &utilexec.ExecResponse{
			// TODO(wallyworld) - use ExitCode() when we support Go 1.12
			// Code:   ps.ProcessState.ExitCode(),
			Code:   exitCode(exitErr),
			Stdout: readBytes(actionOut),
			Stderr: readBytes(actionErr),
		}
		if err := runner.updateActionResults(resp); err != nil {
			return errors.Trace(err)
		}
	}
	return errors.Trace(exitErr)
}

func (runner *runner) startJujucServer(token string, rMode runMode) (*jujuc.Server, error) {
	// Prepare server.
	getCmd := func(ctxId, cmdName string) (cmd.Command, error) {
		if ctxId != runner.context.Id() {
			return nil, errors.Errorf("expected context id %q, got %q", runner.context.Id(), ctxId)
		}
		return jujuc.NewCommand(runner.context, cmdName)
	}

	socket := runner.paths.GetJujucServerSocket(rMode == runOnRemote)
	logger.Debugf("starting jujuc server %s %v", token, socket)
	srv, err := jujuc.NewServer(getCmd, socket, token)
	if err != nil {
		return nil, errors.Annotate(err, "starting jujuc server")
	}
	go srv.Run()
	return srv, nil
}

func (runner *runner) getLogger(hookName string) loggo.Logger {
	return loggo.GetLogger(fmt.Sprintf("unit.%s.%s", runner.context.UnitName(), hookName))
}

type hookProcess struct {
	*os.Process
}

func (p hookProcess) Pid() int {
	return p.Process.Pid
}
