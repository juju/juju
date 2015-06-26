// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package debug

import (
	"bytes"
	"errors"
	"io/ioutil"
	"os"
	"os/exec"

	"github.com/juju/utils/set"
	goyaml "gopkg.in/yaml.v1"
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

# Create welcome message display for the hook environment.
cat > $JUJU_DEBUG/welcome.msg <<END
This is a Juju debug-hooks tmux session. Remember:
1. You need to execute hooks manually if you want them to run for trapped events.
2. When you are finished with an event, you can run 'exit' to close the current window and allow Juju to continue running.
3. CTRL+a is tmux prefix.

More help and info is available in the online documentation:
https://juju.ubuntu.com/docs/authors-hook-debug.html

END

cat > $JUJU_DEBUG/init.sh <<END
#!/bin/bash
cat $JUJU_DEBUG/welcome.msg
trap 'echo \$? > $JUJU_DEBUG/hook_exit_status' EXIT
END
chmod +x $JUJU_DEBUG/init.sh

# Create an internal script which will load the hook environment.
cat > $JUJU_DEBUG/hook.sh <<END
#!/bin/bash
. $JUJU_DEBUG/env.sh
echo \$\$ > $JUJU_DEBUG/hook.pid
exec /bin/bash --noprofile --init-file $JUJU_DEBUG/init.sh
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
typeset -i exitstatus=$(cat $JUJU_DEBUG/hook_exit_status)
exit $exitstatus
`
