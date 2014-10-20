// Copyright 2012-2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package context

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/juju/cmd"
	"github.com/juju/loggo"
	utilexec "github.com/juju/utils/exec"

	"github.com/juju/juju/version"
	"github.com/juju/juju/worker/uniter/context/debug"
	"github.com/juju/juju/worker/uniter/context/jujuc"
)

// Runner is reponsible for invoking commands in a context.
type Runner interface {

	// RunHook executes the hook with the supplied name.
	RunHook(name string) error

	// RunAction executes the action with the supplied name.
	RunAction(name string) error

	// RunCommands executes the supplied script.
	RunCommands(commands string) (*utilexec.ExecResponse, error)
}

// Paths exposes the paths needed by Runner.
type Paths interface {

	// GetToolsDir returns the filesystem path to the dirctory containing
	// the hook tool symlinks.
	GetToolsDir() string

	// GetCharmDir returns the filesystem path to the directory in which
	// the charm is installed.
	GetCharmDir() string

	// GetJujucSocket returns the path to the socket used by the hook tools
	// to communicate back to the executing uniter process. It might be a
	// filesystem path, or it might be abstract.
	GetJujucSocket() string
}

// NewRunner returns a Runner backed by the supplied context and paths.
func NewRunner(context *HookContext, paths Paths) Runner {
	return &runner{context, paths}
}

// runner implements Runner.
type runner struct {
	context *HookContext
	paths   Paths
}

// RunCommands exists to satisfy the Runner interface.
func (runner *runner) RunCommands(commands string) (*utilexec.ExecResponse, error) {
	srv, err := runner.startJujucServer()
	if err != nil {
		return nil, err
	}
	defer srv.Close()

	env := hookVars(runner.context, runner.paths)
	command := utilexec.RunParams{
		Commands:    commands,
		WorkingDir:  runner.paths.GetCharmDir(),
		Environment: env,
	}
	err = command.Run()
	if err != nil {
		return nil, err
	}
	runner.context.setProcess(command.Process())

	// Block and wait for process to finish
	result, err := command.Wait()

	if runner.context.GetRebootPriority() == jujuc.RebootNow {
		// command was killed due to juju-reboot --now
		// clear the error
		err = nil
	}
	return result, runner.context.finalizeContext("run commands", err)
}

// RunAction exists to satisfy the Runner interface.
func (runner *runner) RunAction(actionName string) error {
	if runner.context.actionData == nil {
		return fmt.Errorf("not running an action")
	}
	// If the action had already failed (i.e. from invalid params), we
	// just want to finalize without running it.
	if runner.context.actionData.ActionFailed {
		return runner.context.finalizeContext(actionName, nil)
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

	env := hookVars(runner.context, runner.paths)
	if version.Current.OS == version.Windows {
		// TODO(fwereade): somehow consolidate with utils/exec?
		// We don't do this on the other code path, which uses exec.RunCommands,
		// because that already has handling for windows environment requirements.
		env = mergeEnvironment(env)
	}

	debugctx := debug.NewHooksContext(runner.context.unit.Name())
	if session, _ := debugctx.FindSession(); session != nil && session.MatchHook(hookName) {
		logger.Infof("executing %s via debug-hooks", hookName)
		err = session.RunHook(hookName, runner.paths.GetCharmDir(), env)
	} else {
		err = runner.runCharmHook(hookName, env, charmLocation)
	}
	return runner.context.finalizeContext(hookName, err)
}

func (runner *runner) runCharmHook(hookName string, env []string, charmLocation string) error {
	charmDir := runner.paths.GetCharmDir()
	hook, err := searchHook(charmDir, filepath.Join(charmLocation, hookName))
	if err != nil {
		if IsMissingHookError(err) {
			// Missing hook is perfectly valid, but worth mentioning.
			logger.Infof("skipped %q hook (not implemented)", hookName)
		}
		return err
	}
	hookCmd := hookCommand(hook)
	ps := exec.Command(hookCmd[0], hookCmd[1:]...)
	ps.Env = env
	ps.Dir = charmDir
	outReader, outWriter, err := os.Pipe()
	if err != nil {
		return fmt.Errorf("cannot make logging pipe: %v", err)
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
		// Record the Pid of the hook
		if ps.Process != nil {
			runner.context.setProcess(ps.Process)
		}
		// Block until execution finishes
		err = ps.Wait()
	}
	hookLogger.stop()
	return err
}

func (runner *runner) startJujucServer() (*jujuc.Server, error) {
	// Prepare server.
	getCmd := func(ctxId, cmdName string) (cmd.Command, error) {
		if ctxId != runner.context.Id() {
			return nil, fmt.Errorf("expected context id %q, got %q", runner.context.Id(), ctxId)
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
