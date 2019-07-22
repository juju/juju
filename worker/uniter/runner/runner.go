// Copyright 2012-2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package runner

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"
	"unicode/utf8"

	"github.com/juju/clock"
	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/loggo"
	jujuos "github.com/juju/os"
	utilexec "github.com/juju/utils/exec"

	caasexec "github.com/juju/juju/caas/kubernetes/provider/exec"
	"github.com/juju/juju/core/actions"
	"github.com/juju/juju/worker/common/charmrunner"
	"github.com/juju/juju/worker/uniter/runner/context"
	"github.com/juju/juju/worker/uniter/runner/debug"
	"github.com/juju/juju/worker/uniter/runner/jujuc"
)

var logger = loggo.GetLogger("juju.worker.uniter.runner")

// Runner is responsible for invoking commands in a context.
type Runner interface {

	// Context returns the context against which the runner executes.
	Context() Context

	// RunHook executes the hook with the supplied name.
	RunHook(name string) error

	// RunAction executes the action with the supplied name.
	RunAction(name string, runOnRemote bool) error

	// RunCommands executes the supplied script.
	RunCommands(commands string) (*utilexec.ExecResponse, error)
}

// Context exposes hooks.Context, and additional methods needed by Runner.
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
	result, err := runner.runCommandsWithTimeout(commands, 0, clock.WallClock, false)
	return result, runner.context.Flush("run commands", err)
}

// runCommandsWithTimeout is a helper to abstract common code between run commands and
// juju-run as an action
func (runner *runner) runCommandsWithTimeout(commands string, timeout time.Duration, clock clock.Clock, runOnRemote bool) (*utilexec.ExecResponse, error) {
	srv, err := runner.startJujucServer()
	if err != nil {
		return nil, err
	}
	defer srv.Close()

	env, err := runner.context.HookVars(runner.paths)
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

	executer := runner.runOnMachine
	if runOnRemote {
		executer = runner.runOnRemote
	}
	logger.Criticalf("runJujuRunAction \nenv -> %+v, \nRunParams -> %+v", env, commands)
	return executer(commands, env, clock, cancel)
}

//TODO: for TEST execframework, neeed to plugin this from manifold level into uniter !!!!!!!!!!
func (runner *runner) runOnRemote(commands string, env []string, clock clock.Clock, cancel <-chan struct{}) (*utilexec.ExecResponse, error) {
	c, cfg, err := caasexec.GetInClusterClient()
	if err != nil {
		return nil, errors.Trace(err)
	}
	client := caasexec.New(
		// runner.context.ModelName(),
		"t1",
		c, cfg,
	)

	var stdout, stderr bytes.Buffer

	if err := client.Exec(
		caasexec.ExecParams{
			// TODO: how to get pod name using runner.context.UnitName()
			PodName:  "mariadb-k8s-0",
			Commands: []string{"mkdir", "-p", "/var/lib/juju"},
			Stdout:   &stdout,
			Stderr:   &stderr,
		},
		cancel,
	); err != nil {
		return nil, errors.Trace(err)
	}

	// push files
	// TODO: add a new cmd for checking jujud version, charm/version etc???
	// exec run this new cmd to decide if we need repush files or not.
	for _, sync := range []caasexec.CopyParam{
		{
			Src: caasexec.FileResource{
				Path: "/var/lib/juju/agents/",
			},
			Dest: caasexec.FileResource{
				Path:    "/var/lib/juju/agents/",
				PodName: "mariadb-k8s-0",
			},
		},
		{
			Src: caasexec.FileResource{
				Path: "/var/lib/juju/tools/",
			},
			Dest: caasexec.FileResource{
				Path:    "/var/lib/juju/tools/",
				PodName: "mariadb-k8s-0",
			},
		},
	} {
		if err := client.Copy(sync, cancel); err != nil {
			return nil, errors.Trace(err)
		}
	}

	// TODO: how to get model name properly.
	if err := client.Exec(
		caasexec.ExecParams{
			// TODO: how to get pod name using runner.context.UnitName()
			PodName:    "mariadb-k8s-0",
			Commands:   []string{commands},
			WorkingDir: runner.paths.GetCharmDir(),
			Env:        env,
			Stdout:     &stdout,
			Stderr:     &stderr,
		},
		cancel,
	); err != nil {
		return nil, errors.Trace(err)
	}
	return &utilexec.ExecResponse{
		Stdout: stdout.Bytes(),
		Stderr: stderr.Bytes(),
	}, nil
	// return nil, errors.NotSupportedf("runCommandsWithTimeout cmd -> %q", commands)
}

func (runner *runner) runOnMachine(commands string, env []string, clock clock.Clock, cancel <-chan struct{}) (*utilexec.ExecResponse, error) {
	command := utilexec.RunParams{
		Commands:    commands,
		WorkingDir:  runner.paths.GetCharmDir(),
		Environment: env,
		Clock:       clock,
	}
	err := command.Run()
	if err != nil {
		return nil, err
	}
	// TODO: refactor kill process and implemente kill for caas exec!!!!!!!!!!!!
	runner.context.SetProcess(hookProcess{command.Process()})
	// Block and wait for process to finish
	return command.WaitWithCancel(cancel)
}

// runJujuRunAction is the function that executes when a juju-run action is ran.
func (runner *runner) runJujuRunAction(runOnRemote bool) (err error) {
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

	results, err := runner.runCommandsWithTimeout(command, time.Duration(timeout), clock.WallClock, runOnRemote)
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
func (runner *runner) RunAction(actionName string, runOnRemote bool) error {
	if _, err := runner.context.ActionData(); err != nil {
		return errors.Trace(err)
	}
	if actionName == actions.JujuRunActionName {
		return runner.runJujuRunAction(runOnRemote)
	}
	// run actions on remote workload pod for caas.
	return runner.runCharmHookWithLocation(actionName, "actions", runOnRemote)
}

// RunHook exists to satisfy the Runner interface.
func (runner *runner) RunHook(hookName string) error {
	return runner.runCharmHookWithLocation(hookName, "hooks", false)
}

func (runner *runner) runCharmHookWithLocation(hookName, charmLocation string, runOnRemote bool) (err error) {
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

	defer func() {
		err = runner.context.Flush(hookName, err)
	}()

	debugctx := debug.NewHooksContext(runner.context.UnitName())
	if session, _ := debugctx.FindSession(); session != nil && session.MatchHook(hookName) {
		logger.Infof("executing %s via debug-hooks", hookName)
		return session.RunHook(hookName, runner.paths.GetCharmDir(), env)
	}
	if runOnRemote {
		return runner.runCharmHookOnRemote(hookName, env, charmLocation)
	}
	return runner.runCharmHookOnLocal(hookName, env, charmLocation)
}

func (runner *runner) runCharmHookOnRemote(hookName string, env []string, charmLocation string) error {
	// TODO: use exec framework to run on workload pod !!!!!!!!!
	return errors.NotSupportedf("runCharmHookOnRemote hookName -> %q, env -> %+v, charmLocation -> %q", hookName, env, charmLocation)
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
	ps.Stdout = outWriter
	ps.Stderr = outWriter
	hookLogger := charmrunner.NewHookLogger(runner.getLogger(hookName), outReader)
	go hookLogger.Run()
	err = ps.Start()
	outWriter.Close()
	if err == nil {
		// Record the *os.Process of the hook
		runner.context.SetProcess(hookProcess{ps.Process})
		// Block until execution finishes
		err = ps.Wait()
	}
	hookLogger.Stop()
	return errors.Trace(err)
}

func (runner *runner) startJujucServer() (*jujuc.Server, error) {
	// Prepare server.
	getCmd := func(ctxId, cmdName string) (cmd.Command, error) {
		if ctxId != runner.context.Id() {
			return nil, errors.Errorf("expected context id %q, got %q", runner.context.Id(), ctxId)
		}
		logger.Criticalf("runner.startJujucServer.getCmd cmdName -> %q", cmdName, runner.context.Id())
		return jujuc.NewCommand(runner.context, cmdName)
	}
	logger.Criticalf(
		"runner.context.UnitName() -> %q, runner.context.Id() -> %q, runner.paths.GetJujucSocket() -> %q",
		runner.context.UnitName(),
		runner.context.Id(),
		runner.paths.GetJujucSocket(),
	)
	srv, err := jujuc.NewServer(getCmd, runner.paths.GetJujucSocket())
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
