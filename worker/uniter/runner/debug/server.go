// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package debug

import (
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"github.com/juju/utils"
	goyaml "gopkg.in/yaml.v2"
)

// ServerSession represents a "juju debug-hooks" session.
type ServerSession struct {
	*HooksContext
	hooks   set.Strings
	debugAt string

	output io.Writer
}

// MatchHook returns true if the specified hook name matches
// the hook specified by the debug-hooks client.
func (s *ServerSession) MatchHook(hookName string) bool {
	return s.hooks.IsEmpty() || s.hooks.Contains(hookName)
}

// DebugAt returns the location for the charm to stop for debugging, if it is set.
func (s *ServerSession) DebugAt() string {
	return s.debugAt
}

// waitClientExit executes flock, waiting for the SSH client to exit.
// This is a var so it can be replaced for testing.
var waitClientExit = func(s *ServerSession) {
	path := s.ClientExitFileLock()
	exec.Command("flock", path, "-c", "true").Run()
}

// RunHook "runs" the hook with the specified name via debug-hooks. The hookRunner
// parameters specifies the name of the binary that users can invoke to handle
// the hook. When using the legacy hook system, hookRunner will be equal to
// the hookName; otherwise, it will point to a script that acts as the dispatcher
// for all hooks/actions.
func (s *ServerSession) RunHook(hookName, charmDir string, env []string, hookRunner string) error {
	debugDir, err := ioutil.TempDir("", "juju-debug-hooks-")
	if err != nil {
		return errors.Trace(err)
	}
	defer func() { _ = os.RemoveAll(debugDir) }()
	help := buildRunHookCmd(hookName, hookRunner)
	if err := s.writeDebugFiles(debugDir, help, hookRunner); err != nil {
		return errors.Trace(err)
	}

	env = utils.Setenv(env, "JUJU_DEBUG="+debugDir)
	if s.debugAt != "" {
		env = utils.Setenv(env, "JUJU_DEBUG_AT="+s.debugAt)
	}

	cmd := exec.Command("/bin/bash", "-s")
	cmd.Env = env
	cmd.Dir = charmDir
	cmd.Stdin = bytes.NewBufferString(debugHooksServerScript)
	if s.output != nil {
		cmd.Stdout = s.output
		cmd.Stderr = s.output
	}
	if err := cmd.Start(); err != nil {
		return err
	}
	go func(proc *os.Process) {
		// Wait for the SSH client to exit (i.e. release the flock),
		// then kill the server hook process in case the client
		// exited uncleanly.
		waitClientExit(s)
		_ = proc.Kill()
	}(cmd.Process)
	return cmd.Wait()
}

func buildRunHookCmd(hookName, hookRunner string) string {
	if hookName == filepath.Base(hookRunner) {
		return "./$JUJU_DISPATCH_PATH"
	}
	return "./" + hookRunner
}

func (s *ServerSession) writeDebugFiles(debugDir, help, hookRunner string) error {
	// hook.sh does not inherit environment variables,
	// so we must insert the path to the directory
	// containing env.sh for it to source.
	debugHooksHookScript := strings.Replace(strings.Replace(
		debugHooksHookScript,
		"__JUJU_DEBUG__", debugDir, -1,
	), "__JUJU_HOOK_RUNNER__", hookRunner, -1)

	type file struct {
		filename string
		contents string
		mode     os.FileMode
	}
	files := []file{
		{"welcome.msg", fmt.Sprintf(debugHooksWelcomeMessage, help), 0644},
		{"init.sh", debugHooksInitScript, 0755},
		{"hook.sh", debugHooksHookScript, 0755},
	}
	for _, file := range files {
		if err := ioutil.WriteFile(
			filepath.Join(debugDir, file.filename),
			[]byte(file.contents),
			file.mode,
		); err != nil {
			return errors.Annotatef(err, "writing %q", file.filename)
		}
	}
	return nil
}

// FindSession attempts to find a debug hooks session for the unit specified
// in the context, and returns a new ServerSession structure for it.
func (c *HooksContext) FindSession() (*ServerSession, error) {
	cmd := exec.Command("tmux", "has-session", "-t", c.tmuxSessionName())
	out, err := cmd.CombinedOutput()
	if err != nil {
		if len(out) != 0 {
			return nil, errors.New(string(out))
		} else {
			return nil, err
		}
	}
	// Parse the debug-hooks file for an optional hook name.
	data, err := ioutil.ReadFile(c.ClientFileLock())
	if err != nil {
		return nil, err
	}
	var args hookArgs
	err = goyaml.Unmarshal(data, &args)
	if err != nil {
		return nil, err
	}
	hooks := set.NewStrings(args.Hooks...)
	session := &ServerSession{HooksContext: c, hooks: hooks, debugAt: args.DebugAt}
	return session, nil
}

const debugHooksServerScript = `set -e
exec > $JUJU_DEBUG/debug.log >&1

# Set a useful prompt.
export PS1="$JUJU_UNIT_NAME:$JUJU_DISPATCH_PATH % "

# Save environment variables and export them for sourcing.
FILTER='^\(LS_COLORS\|LESSOPEN\|LESSCLOSE\|PWD\)='
export | grep -v $FILTER > $JUJU_DEBUG/env.sh

if [ -z "$JUJU_HOOK_NAME" ] ; then
  window_name="$JUJU_DISPATCH_PATH"
else
  window_name="$JUJU_HOOK_NAME"
fi
tmux new-window -t $JUJU_UNIT_NAME -n $window_name "$JUJU_DEBUG/hook.sh"

# If we exit for whatever reason, kill the hook shell.
exit_handler() {
    if [ -f $JUJU_DEBUG/hook.pid ]; then
        kill -9 $(cat $JUJU_DEBUG/hook.pid) 2>/dev/null || true
    fi
}
trap exit_handler EXIT

# Wait for the hook shell to start, and then wait for it to exit.
while [ ! -f $JUJU_DEBUG/hook.pid ]; do
    sleep 1
done
HOOK_PID=$(cat $JUJU_DEBUG/hook.pid)
while kill -0 "$HOOK_PID" 2> /dev/null; do
    sleep 1
done
typeset -i exitstatus=$(cat $JUJU_DEBUG/hook_exit_status)
exit $exitstatus
`

const debugHooksWelcomeMessage = `This is a Juju debug-hooks tmux session. Remember:
1. You need to execute hooks/actions manually if you want them to run for trapped events.
2. When you are finished with an event, you can run 'exit' to close the current window and allow Juju to continue processing
new events for this unit without exiting a current debug-session.
3. To run an action or hook and end the debugging session avoiding processing any more events manually, use:

%s
tmux kill-session -t $JUJU_UNIT_NAME # or, equivalently, CTRL+a d

4. CTRL+a is tmux prefix.

More help and info is available in the online documentation:
https://discourse.jujucharms.com/t/debugging-charm-hooks

`

const debugHooksInitScript = `#!/bin/bash
envsubst < $JUJU_DEBUG/welcome.msg
trap 'echo $? > $JUJU_DEBUG/hook_exit_status' EXIT
`

// debugHooksHookScript is the shell script that tmux spawns instead of running the normal hook.
// In a debug session, we bring in the environment and record our scripts PID as the
// hook.pid that the rest of the server is waiting for. Without BREAKPOINT, we then exec an
// interactive shell with an init.sh that displays a welcome message and traps its exit code into
// hook_exit_status.
// With JUJU_DEBUG_AT, we just exec the hook directly, and record its exit status before exit.
// It is the responsibility of the code handling JUJU_DEBUG_AT to handle prompting.
const debugHooksHookScript = `#!/bin/bash
. __JUJU_DEBUG__/env.sh
echo $$ > $JUJU_DEBUG/hook.pid
if [ -z "$JUJU_DEBUG_AT" ] ; then
	exec /bin/bash --noprofile --init-file $JUJU_DEBUG/init.sh
elif [ ! -x "__JUJU_HOOK_RUNNER__" ] ; then
	juju-log --log-level INFO "debugging is enabled, but no handler for $JUJU_HOOK_NAME, skipping"
	echo 0 > $JUJU_DEBUG/hook_exit_status
else
	juju-log --log-level INFO "debug running __JUJU_HOOK_RUNNER__ for $JUJU_HOOK_NAME"
	__JUJU_HOOK_RUNNER__
	echo $? > $JUJU_DEBUG/hook_exit_status
fi
`
