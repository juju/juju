// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"encoding/base64"
	"errors"
	"fmt"
	"os/exec"
	"strings"

	"launchpad.net/juju-core/cmd"
	"launchpad.net/juju-core/juju"
)

// DebugHooksCommand is responsible for launching a ssh shell on a given unit or machine.
type DebugHooksCommand struct {
	SSHCommon
}

const debugHooksDoc = `
Interactively debug a hook remotely on a service unit.
`

func (c *DebugHooksCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "debug-hooks",
		Args:    "<unit name> [hook names]",
		Purpose: "launch an tmux session to debug a hook",
		Doc:     debugHooksDoc,
	}
}

func (c *DebugHooksCommand) Init(args []string) error {
	if len(args) < 1 {
		return errors.New("no unit name specified")
	}
	c.Target = args[0]
	return nil
}

// Run resolves c.Target to a machine, to the address of a i
// machine or unit forks ssh passing any arguments provided.
func (c *DebugHooksCommand) Run(ctx *cmd.Context) error {
	var err error
	c.Conn, err = juju.NewConnFromName(c.EnvName)
	if err != nil {
		return err
	}
	defer c.Close()
	host, err := c.hostFromTarget(c.Target)
	if err != nil {
		return err
	}
	script := strings.Replace(tmpl, "{unit_name}", c.Target, -1)

	args := []string{"-l", "ubuntu", "-t", "-o", "StrictHostKeyChecking no", "-o", "PasswordAuthentication no", host, "--"}
	script = base64.StdEncoding.EncodeToString([]byte(script))
	args = append(args, fmt.Sprintf("sudo /bin/bash -c '%s'", fmt.Sprintf(`F=$(mktemp); echo %s | base64 -d > $F; . $F`, script)))
	cmd := exec.Command("ssh", args...)
	cmd.Stdin = ctx.Stdin
	cmd.Stdout = ctx.Stdout
	cmd.Stderr = ctx.Stderr
	c.Close()
	return cmd.Run()
}

const tmpl = `
# Wait for tmux to be installed.
while [ ! -f /usr/bin/tmux ]; do
    sleep 1
done

if [ ! -f ~/.tmux.conf ]; then
        if [ -f /usr/share/byobu/profiles/tmux ]; then
                # Use byobu/tmux profile for familiar keybindings and branding
                echo "source-file /usr/share/byobu/profiles/tmux" > ~/.tmux.conf
        else
                # Otherwise, use the legacy juju/tmux configuration
                cat > ~/.tmux.conf <<END

# Status bar
set-option -g status-bg black
set-option -g status-fg white

set-window-option -g window-status-current-bg red
set-window-option -g window-status-current-attr bright

set-option -g status-right ''

# Panes
set-option -g pane-border-fg white
set-option -g pane-active-border-fg white

# Monitor activity on windows
set-window-option -g monitor-activity on

# Screen bindings, since people are more familiar with that.
set-option -g prefix C-a
bind C-a last-window
bind a send-key C-a

bind | split-window -h
bind - split-window -v

# Fix CTRL-PGUP/PGDOWN for vim
set-window-option -g xterm-keys on

# Prevent ESC key from adding delay and breaking Vim's ESC > arrow key
set-option -s escape-time 0

END
        fi
fi

# The beauty below is a workaround for a bug in tmux (1.5 in Oneiric) or
# epoll that doesn't support /dev/null or whatever.  Without it the
# command hangs.
tmux new-session -d -s {unit_name} 2>&1 | cat > /dev/null || true
tmux attach -t {unit_name}
`
