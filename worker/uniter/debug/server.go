// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package debug

import (
	"bytes"
	"errors"
	"io/ioutil"
	"os"
	"os/exec"
	"strings"
)

// DebugHooksServerSession represents a "juju debug-hooks" session.
type DebugHooksServerSession struct {
	*DebugHooksContext
	hooks map[string]bool
}

// MatchHook returns true if the specified hook name matches
// the hook specified by the debug-hooks client.
func (s *DebugHooksServerSession) MatchHook(hookName string) bool {
	return len(s.hooks) == 0 || s.hooks[hookName]
}

// RunHook "runs" the hook with the specified name via debug-hooks.
func (s *DebugHooksServerSession) RunHook(hookName, charmDir string, env []string) error {
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
		path := s.ClientExitFileLock()
		exec.Command("flock", path, "-c", "true").Run()
		proc.Kill()
	}(cmd.Process)
	return cmd.Wait()
}

// FindSession attempts to find a debug hooks session for the unit specified
// in the context, and returns a new DebugHooksServerSession structure for it.
func (c *DebugHooksContext) FindSession() (*DebugHooksServerSession, error) {
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
	hooks := make(map[string]bool)
	for _, hook := range strings.Fields(string(data)) {
		hooks[hook] = true
	}
	session := &DebugHooksServerSession{c, hooks}
	return session, nil
}

const debugHooksServerScript = `set -e
export JUJU_DEBUG=$(mktemp -d)
exec > $JUJU_DEBUG/debug.log >&1

# Save environment variables and export them for sourcing.
FILTER='^\(LS_COLORS\|LESSOPEN\|LESSCLOSE\|PWD\)='
env | grep -v $FILTER > $JUJU_DEBUG/env.sh
sed -i 's/^/export /' $JUJU_DEBUG/env.sh

# Create an internal script which will load the hook environment.
cat > $JUJU_DEBUG/hook.sh <<END
#!/bin/bash
. $JUJU_DEBUG/env.sh
echo \$\$ > $JUJU_DEBUG/hook.pid
exec /bin/bash
END
chmod +x $JUJU_DEBUG/hook.sh

# If the session already exists, the ssh command won the race, so just use it.
# The beauty below is a workaround for a bug in tmux (1.5 in Oneiric) or
# epoll that doesn't support /dev/null or whatever.  Without it the
# command hangs.
tmux new-session -d -s $JUJU_UNIT_NAME 2>&1 | cat > /dev/null || true
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
