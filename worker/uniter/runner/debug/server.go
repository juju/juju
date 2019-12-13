// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package debug

import (
	"bytes"
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
	hooks set.Strings

	output io.Writer
}

// MatchHook returns true if the specified hook name matches
// the hook specified by the debug-hooks client.
func (s *ServerSession) MatchHook(hookName string) bool {
	return s.hooks.IsEmpty() || s.hooks.Contains(hookName)
}

// waitClientExit executes flock, waiting for the SSH client to exit.
// This is a var so it can be replaced for testing.
var waitClientExit = func(s *ServerSession) {
	path := s.ClientExitFileLock()
	exec.Command("flock", path, "-c", "true").Run()
}

// RunHook "runs" the hook with the specified name via debug-hooks.
func (s *ServerSession) RunHook(hookName, charmDir string, env []string) error {
	debugDir, err := ioutil.TempDir("", "juju-debug-hooks-")
	if err != nil {
		return errors.Trace(err)
	}
	defer os.RemoveAll(debugDir)
	if err := s.writeDebugFiles(debugDir); err != nil {
		return errors.Trace(err)
	}

	// TODO add JUJU_DISPATCH_HOOK if needed.
	env = utils.Setenv(env, "JUJU_HOOK_NAME="+hookName)
	env = utils.Setenv(env, "JUJU_DEBUG="+debugDir)

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
		proc.Kill()
	}(cmd.Process)
	return cmd.Wait()
}

func (s *ServerSession) writeDebugFiles(debugDir string) error {
	// hook.sh does not inherit environment variables,
	// so we must insert the path to the directory
	// containing env.sh for it to source.
	debugHooksHookScript := strings.Replace(
		debugHooksHookScript,
		"__JUJU_DEBUG__", debugDir, -1,
	)

	type file struct {
		filename string
		contents string
		mode     os.FileMode
	}
	files := []file{
		{"welcome.msg", debugHooksWelcomeMessage, 0644},
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
	session := &ServerSession{HooksContext: c, hooks: hooks}
	return session, nil
}

const debugHooksServerScript = `set -e
exec > $JUJU_DEBUG/debug.log >&1

# Set a useful prompt.
export PS1="$JUJU_UNIT_NAME:$JUJU_HOOK_NAME % "

# Save environment variables and export them for sourcing.
FILTER='^\(LS_COLORS\|LESSOPEN\|LESSCLOSE\|PWD\)='
export | grep -v $FILTER > $JUJU_DEBUG/env.sh

tmux new-window -t $JUJU_UNIT_NAME -n $JUJU_HOOK_NAME "$JUJU_DEBUG/hook.sh"

# If we exit for whatever reason, kill the hook shell.
exit_handler() {
    if [ -f $JUJU_DEBUG/hook.pid ]; then
        kill -9 $(cat $JUJU_DEBUG/hook.pid) || true
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

./hooks/$JUJU_HOOK_NAME # or, equivalently, ./actions/$JUJU_HOOK_NAME
tmux kill-session -t $JUJU_UNIT_NAME # or, equivalently, CTRL+a d

4. CTRL+a is tmux prefix.

More help and info is available in the online documentation:
https://discourse.jujucharms.com/t/debugging-charm-hooks

`

const debugHooksInitScript = `#!/bin/bash
envsubst < $JUJU_DEBUG/welcome.msg
trap 'echo $? > $JUJU_DEBUG/hook_exit_status' EXIT
`

const debugHooksHookScript = `#!/bin/bash
. __JUJU_DEBUG__/env.sh
echo $$ > $JUJU_DEBUG/hook.pid
exec /bin/bash --noprofile --init-file $JUJU_DEBUG/init.sh
`
