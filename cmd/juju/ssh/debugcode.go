// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package ssh

import (
	"github.com/juju/gnuflag"
	"github.com/juju/retry"

	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/internal/cmd"
	"github.com/juju/juju/internal/network/ssh"
)

const usageDebugCodeExamples = `
Debug all hooks and actions of unit '0':

    juju debug-code mysql/0

Debug all hooks and actions of the leader:

    juju debug-code mysql/leader

Debug the 'config-changed' hook of unit '1':

    juju debug-code mysql/1 config-changed

Debug the 'pull-site' action and 'update-status' hook:

    juju debug-code hello-kubecon/0 pull-site update-status

Debug the 'leader-elected' hook and set 'JUJU_DEBUG_AT' variable to 'hook':

    juju debug-code --at=hook mysql/0 leader-elected
`

func NewDebugCodeCommand(hostChecker ssh.ReachableChecker, retryStrategy retry.CallArgs, publicKeyRetryStrategy retry.CallArgs) cmd.Command {
	c := new(debugCodeCommand)
	c.hostChecker = hostChecker
	c.retryStrategy = retryStrategy
	c.publicKeyRetryStrategy = publicKeyRetryStrategy
	return modelcmd.Wrap(c)
}

// debugCodeCommand connects via SSH to a running unit, and drops into a tmux shell,
// prepared to debug hooks/actions as they fire.
type debugCodeCommand struct {
	debugHooksCommand
	debugAt string
}

const debugCodeDoc = `
The command launches a tmux session that will intercept matching hooks and/or
actions.

Initially, the tmux session will take you to '/var/lib/juju' or '/home/ubuntu'.
As soon as a matching hook or action is fired, the hook or action is executed
and the JUJU_DEBUG_AT variable is set. Charms implementing support for this
should set debug breakpoints based on the environment variable. Charms written
with the Ops library automatically provide support for this.

Valid unit identifiers are:
  a standard unit ID, such as mysql/0 or;
  leader syntax of the form <application>/leader, such as mysql/leader.

If no hook or action is specified, all hooks and actions will be intercepted.

See the "juju help ssh" for information about SSH related options
accepted by the debug-code command.
`

func (c *debugCodeCommand) Info() *cmd.Info {
	return jujucmd.Info(&cmd.Info{
		Name:     "debug-code",
		Args:     "<unit name> [hook or action names]",
		Purpose:  "Launch a tmux session to debug hooks and/or actions.",
		Doc:      debugCodeDoc,
		Examples: usageDebugCodeExamples,
		SeeAlso: []string{
			"ssh",
			"debug-hooks",
		},
	})
}

func (c *debugCodeCommand) Init(args []string) error {
	return c.debugHooksCommand.Init(args)
}

func (c *debugCodeCommand) SetFlags(f *gnuflag.FlagSet) {
	c.debugHooksCommand.SetFlags(f)
	f.StringVar(&c.debugAt, "at", "all",
		"will set the JUJU_DEBUG_AT environment variable to this value, which will\n"+
			"then be interpreted by the charm for where you want to stop, defaults to 'all'")
}

// Run ensures c.Target is a unit, and resolves its address,
// and connects to it via SSH to execute the debug-hooks
// script.
func (c *debugCodeCommand) Run(ctx *cmd.Context) error {
	if err := c.initAPIs(ctx); err != nil {
		return err
	}
	defer c.closeAPIs()
	return c.commonRun(ctx, c.provider.getTarget(), c.hooks, c.debugAt)
}
