// Copyright 2012-2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package runner

import (
	"encoding/base64"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"
	"unicode/utf8"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/utils/clock"
	utilexec "github.com/juju/utils/exec"

	"github.com/juju/juju/core/actions"
	"github.com/juju/juju/worker/uniter/runner/context"
	"github.com/juju/juju/worker/uniter/runner/debug"
	"github.com/juju/juju/worker/uniter/runner/jujuc"
	jujuos "github.com/juju/utils/os"
)

var logger = loggo.GetLogger("juju.worker.uniter.runner")

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

// Context exposes jujuc.Context, and additional methods needed by Runner.
type Context interface {
	jujuc.Context
	Id() string
	HookVars(paths context.Paths) ([]string, error)
	ActionData() (*context.ActionData, error)
	SetProcess(process context.HookProcess)
	HasExecutionSetUnitStatus() bool
	ResetExecutionSetUnitStatus()

	Prepare() error
	Flush(badge string, failure error) error
}

// NewRunner returns a Runner backed by the supplied context and paths.
func NewRunner(context Context, paths context.Paths) Runner {
	return &runner{context, paths}
}

// runner implements Runner.
type runner struct {
	context Context
	paths   context.Paths
}

func (runner *runner) Context() Context {
	return runner.context
}

// RunCommands exists to satisfy the Runner interface.
func (runner *runner) RunCommands(commands string) (*utilexec.ExecResponse, error) {
	result, err := runner.runCommandsWithTimeout(commands, 0, clock.WallClock)
	return result, runner.context.Flush("run commands", err)
}

// runCommandsWithTimeout is a helper to abstract common code between run commands and
// juju-run as an action
func (runner *runner) runCommandsWithTimeout(commands string, timeout time.Duration, clock clock.Clock) (*utilexec.ExecResponse, error) {
	srv, err := runner.startJujucServer()
	if err != nil {
		return nil, err
	}
	defer srv.Close()

	env, err := runner.context.HookVars(runner.paths)
	if err != nil {
		return nil, errors.Trace(err)
	}
	command := utilexec.RunParams{
		Commands:    commands,
		WorkingDir:  runner.paths.GetCharmDir(),
		Environment: env,
		Clock:       clock,
	}

	err = command.Run()
	if err != nil {
		return nil, err
	}
	runner.context.SetProcess(hookProcess{command.Process()})

	var cancel chan struct{}
	if timeout != 0 {
		cancel = make(chan struct{})
		go func() {
			<-clock.After(timeout)
			close(cancel)
		}()
	}

	// Block and wait for process to finish
	return command.WaitWithCancel(cancel)
}

// runJujuRunAction is the function that executes when a juju-run action is ran.
func (runner *runner) runJujuRunAction() (err error) {
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

	results, err := runner.runCommandsWithTimeout(command, time.Duration(timeout), clock.WallClock)

	if err != nil {
		return runner.context.Flush("juju-run", err)
	}

	if err := runner.updateActionResults(results); err != nil {
		return runner.context.Flush("juju-run", err)
	}

	return runner.context.Flush("juju-run", nil)
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
	if err := runner.context.UpdateActionResults([]string{"Code"}, fmt.Sprintf("%d", results.Code)); err != nil {
		return errors.Trace(err)
	}

	stdout, encoding := encodeBytes(results.Stdout)
	if err := runner.context.UpdateActionResults([]string{"Stdout"}, stdout); err != nil {
		return errors.Trace(err)
	}
	if encoding != "utf8" {
		if err := runner.context.UpdateActionResults([]string{"StdoutEncoding"}, encoding); err != nil {
			return errors.Trace(err)
		}
	}

	stderr, encoding := encodeBytes(results.Stderr)
	if err := runner.context.UpdateActionResults([]string{"Stderr"}, stderr); err != nil {
		return errors.Trace(err)
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
	return runner.runCharmHookWithLocation(actionName, "actions")
}

// RunHook exists to satisfy the Runner interface.
func (runner *runner) RunHook(hookName string) error {
	return runner.runCharmHookWithLocation(hookName, "hooks")
}

func (runner *runner) runCharmHookWithLocation(hookName, charmLocation string) error {
	srv, err := runner.startJujucServer()
	if err != nil {
		return err
	}
	defer srv.Close()

	env, err := runner.context.HookVars(runner.paths)
	if err != nil {
		return errors.Trace(err)
	}
	if jujuos.HostOS() == jujuos.Windows {
		// TODO(fwereade): somehow consolidate with utils/exec?
		// We don't do this on the other code path, which uses exec.RunCommands,
		// because that already has handling for windows environment requirements.
		env = mergeWindowsEnvironment(env, os.Environ())
	}

	debugctx := debug.NewHooksContext(runner.context.UnitName())
	if session, _ := debugctx.FindSession(); session != nil && session.MatchHook(hookName) {
		logger.Infof("executing %s via debug-hooks", hookName)
		err = session.RunHook(hookName, runner.paths.GetCharmDir(), env)
	} else {
		err = runner.runCharmHook(hookName, env, charmLocation)
	}
	return runner.context.Flush(hookName, err)
}

func (runner *runner) runCharmHook(hookName string, env []string, charmLocation string) error {
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
	ps.Stdout = outWriter
	ps.Stderr = outWriter
	hookLogger := &hookLogger{
		r:      outReader,
		done:   make(chan struct{}),
		logger: runner.getLogger(hookName),
	}
	go hookLogger.run()
	err = ps.Start()
	outWriter.Close()
	if err == nil {
		// Record the *os.Process of the hook
		runner.context.SetProcess(hookProcess{ps.Process})
		// Block until execution finishes
		err = ps.Wait()
	}
	hookLogger.stop()
	return errors.Trace(err)
}

func (runner *runner) startJujucServer() (*jujuc.Server, error) {
	// Prepare server.
	getCmd := func(ctxId, cmdName string) (cmd.Command, error) {
		if ctxId != runner.context.Id() {
			return nil, errors.Errorf("expected context id %q, got %q", runner.context.Id(), ctxId)
		}
		return jujuc.NewCommand(runner.context, cmdName)
	}
	srv, err := jujuc.NewServer(getCmd, runner.paths.GetJujucSocket())
	if err != nil {
		return nil, err
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
