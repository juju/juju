// Copyright 2012-2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package context

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/juju/loggo"
	utilexec "github.com/juju/utils/exec"

	"github.com/juju/juju/version"
	unitdebug "github.com/juju/juju/worker/uniter/debug"
)

var windowsSuffixOrder = []string{
	".ps1",
	".cmd",
	".bat",
	".exe",
}

// mergeEnvironment takes in a string array representing the desired environment
// and merges it with the current environment. On Windows, clearing the environment,
// or having missing environment variables, may lead to standard go packages not working
// (os.TempDir relies on $env:TEMP), and powershell erroring out
// Currently this function is only used for windows
func mergeEnvironment(env []string) []string {
	if env == nil {
		return nil
	}
	m := map[string]string{}
	var tmpEnv []string
	for _, val := range os.Environ() {
		varSplit := strings.SplitN(val, "=", 2)
		m[varSplit[0]] = varSplit[1]
	}

	for _, val := range env {
		varSplit := strings.SplitN(val, "=", 2)
		m[varSplit[0]] = varSplit[1]
	}

	for key, val := range m {
		tmpEnv = append(tmpEnv, key+"="+val)
	}

	return tmpEnv
}

// windowsEnv adds windows specific environment variables. PSModulePath
// helps hooks use normal imports instead of dot sourcing modules
// its a convenience variable. The PATH variable delimiter is
// a semicolon instead of a colon
func (ctx *HookContext) windowsEnv(charmDir, toolsDir string) []string {
	charmModules := filepath.Join(charmDir, "Modules")
	hookModules := filepath.Join(charmDir, "hooks", "Modules")
	env := []string{
		"Path=" + filepath.FromSlash(toolsDir) + ";" + os.Getenv("Path"),
		"PSModulePath=" + os.Getenv("PSModulePath") + ";" + charmModules + ";" + hookModules,
	}
	return mergeEnvironment(env)
}

func (ctx *HookContext) ubuntuEnv(toolsDir string) []string {
	env := []string{
		"APT_LISTCHANGES_FRONTEND=none",
		"DEBIAN_FRONTEND=noninteractive",
		"PATH=" + toolsDir + ":" + os.Getenv("PATH"),
	}
	return env
}

func (ctx *HookContext) osDependentEnvVars(charmDir, toolsDir string) []string {
	switch version.Current.OS {
	case version.Windows:
		return ctx.windowsEnv(charmDir, toolsDir)
	default:
		return ctx.ubuntuEnv(toolsDir)
	}
}

// hookVars returns an os.Environ-style list of strings necessary to run a hook
// such that it can know what environment it's operating in, and can call back
// into ctx.
func (ctx *HookContext) hookVars(charmDir, toolsDir, socketPath string) []string {
	// TODO(binary132): add Action env variables: JUJU_ACTION_NAME,
	// JUJU_ACTION_UUID, ...
	vars := []string{
		"CHARM_DIR=" + charmDir,
		"JUJU_CONTEXT_ID=" + ctx.id,
		"JUJU_AGENT_SOCKET=" + socketPath,
		"JUJU_UNIT_NAME=" + ctx.unit.Name(),
		"JUJU_ENV_UUID=" + ctx.uuid,
		"JUJU_ENV_NAME=" + ctx.envName,
		"JUJU_API_ADDRESSES=" + strings.Join(ctx.apiAddrs, " "),
	}
	osVars := ctx.osDependentEnvVars(charmDir, toolsDir)
	vars = append(vars, osVars...)

	if r, found := ctx.HookRelation(); found {
		vars = append(vars, "JUJU_RELATION="+r.Name())
		vars = append(vars, "JUJU_RELATION_ID="+r.FakeId())
		name, _ := ctx.RemoteUnitName()
		vars = append(vars, "JUJU_REMOTE_UNIT="+name)
	}
	vars = append(vars, ctx.proxySettings.AsEnvironmentValues()...)
	vars = append(vars, ctx.meterStatusEnvVars()...)
	return vars
}

// meterStatusEnvVars returns meter status environment variables if the meter
// status is set.
func (ctx *HookContext) meterStatusEnvVars() []string {
	if ctx.meterStatus != nil {
		return []string{
			fmt.Sprintf("JUJU_METER_STATUS=%s", ctx.meterStatus.code),
			fmt.Sprintf("JUJU_METER_INFO=%s", ctx.meterStatus.info)}
	}
	return nil
}

// RunCommands executes the commands in an environment which allows it to to
// call back into the hook context to execute jujuc tools.
func (ctx *HookContext) RunCommands(commands, charmDir, toolsDir, socketPath string) (*utilexec.ExecResponse, error) {
	env := ctx.hookVars(charmDir, toolsDir, socketPath)
	result, err := utilexec.RunCommands(
		utilexec.RunParams{
			Commands:    commands,
			WorkingDir:  charmDir,
			Environment: env})
	return result, ctx.finalizeContext("run commands", err)
}

func (ctx *HookContext) GetLogger(hookName string) loggo.Logger {
	return loggo.GetLogger(fmt.Sprintf("unit.%s.%s", ctx.UnitName(), hookName))
}

// RunAction executes a hook from the charm's actions in an environment which
// allows it to to call back into the hook context to execute jujuc tools.
func (ctx *HookContext) RunAction(hookName, charmDir, toolsDir, socketPath string) error {
	if ctx.actionData == nil {
		return fmt.Errorf("not running an action")
	}
	// If the action had already failed (i.e. from invalid params), we
	// just want to finalize without running it.
	if ctx.actionData.ActionFailed {
		return ctx.finalizeContext(hookName, nil)
	}
	return ctx.runCharmHookWithLocation(hookName, "actions", charmDir, toolsDir, socketPath)
}

// RunHook executes a built-in hook in an environment which allows it to to
// call back into the hook context to execute jujuc tools.
func (ctx *HookContext) RunHook(hookName, charmDir, toolsDir, socketPath string) error {
	return ctx.runCharmHookWithLocation(hookName, "hooks", charmDir, toolsDir, socketPath)
}

func (ctx *HookContext) runCharmHookWithLocation(hookName, charmLocation, charmDir, toolsDir, socketPath string) error {
	var err error
	env := ctx.hookVars(charmDir, toolsDir, socketPath)
	debugctx := unitdebug.NewHooksContext(ctx.unit.Name())
	if session, _ := debugctx.FindSession(); session != nil && session.MatchHook(hookName) {
		logger.Infof("executing %s via debug-hooks", hookName)
		err = session.RunHook(hookName, charmDir, env)
	} else {
		err = ctx.runCharmHook(hookName, charmDir, env, charmLocation)
	}
	return ctx.finalizeContext(hookName, err)
}

func lookPath(hook string) (string, error) {
	hookFile, err := exec.LookPath(hook)
	if err != nil {
		if ee, ok := err.(*exec.Error); ok && os.IsNotExist(ee.Err) {
			return "", &missingHookError{hook}
		}
		return "", err
	}
	return hookFile, nil
}

// searchHook will search, in order, hooks suffixed with extensions
// in windowsSuffixOrder. As windows cares about extensions to determine
// how to execute a file, we will allow several suffixes, with powershell
// being default.
func searchHook(charmDir, hook string) (string, error) {
	hookFile := filepath.Join(charmDir, hook)
	if version.Current.OS != version.Windows {
		// we are not running on windows,
		// there is no need to look for suffixed hooks
		return lookPath(hookFile)
	}
	for _, val := range windowsSuffixOrder {
		file := fmt.Sprintf("%s%s", hookFile, val)
		foundHook, err := lookPath(file)
		if err != nil {
			if IsMissingHookError(err) {
				// look for next suffix
				continue
			}
			return "", err
		}
		return foundHook, nil
	}
	return "", &missingHookError{hook}
}

// hookCommand constructs an appropriate command to be passed to
// exec.Command(). The exec package uses cmd.exe as default on windows
// cmd.exe does not know how to execute ps1 files by default, and
// powershell needs a few flags to allow execution (-ExecutionPolicy)
// and propagate error levels (-File). .cmd and .bat files can be run directly
func hookCommand(hook string) []string {
	if version.Current.OS != version.Windows {
		// we are not running on windows,
		// just return the hook name
		return []string{hook}
	}
	if strings.HasSuffix(hook, ".ps1") {
		return []string{
			"powershell.exe",
			"-NonInteractive",
			"-ExecutionPolicy",
			"RemoteSigned",
			"-File",
			hook,
		}
	}
	return []string{hook}
}

func (ctx *HookContext) runCharmHook(hookName, charmDir string, env []string, charmLocation string) error {
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
		logger: ctx.GetLogger(hookName),
	}
	go hookLogger.run()
	err = ps.Start()
	outWriter.Close()
	if err == nil {
		err = ps.Wait()
	}
	hookLogger.stop()
	return err
}
