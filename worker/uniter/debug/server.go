// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package debug

import (
	"bytes"
	"errors"
	"io/ioutil"
	"os"
	"os/exec"

	"launchpad.net/goyaml"

	"launchpad.net/juju-core/utils/set"
)

// ServerSession represents a "juju debug-hooks" session.
type ServerSession struct {
	*HooksContext
	hooks set.Strings
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
	env = append(env, "JUJU_HOOK_NAME="+hookName)
	cmd := exec.Command("/bin/bash", "-s")
	cmd.Env = env
	cmd.Dir = charmDir
	cmd.Stdin = bytes.NewBufferString(debugHooksServerScript)
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
	session := &ServerSession{c, hooks}
	return session, nil
}

const debugHooksServerScript = `set -e
export JUJU_DEBUG=$(mktemp -d)
exec > $JUJU_DEBUG/debug.log >&1

# Set a useful prompt.
export PS1="$JUJU_UNIT_NAME:$JUJU_HOOK_NAME % "

# Save environment variables and export them for sourcing.
FILTER='^\(LS_COLORS\|LESSOPEN\|LESSCLOSE\|PWD\)='
export | grep -v $FILTER > $JUJU_DEBUG/env.sh

# Create an internal script which will load the hook environment.
cat > $JUJU_DEBUG/hook.sh <<END
#!/bin/bash
. $JUJU_DEBUG/env.sh
echo \$\$ > $JUJU_DEBUG/hook.pid
exec /bin/bash --noprofile --norc
END
chmod +x $JUJU_DEBUG/hook.sh

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
`
