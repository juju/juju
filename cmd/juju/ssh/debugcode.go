// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package ssh

import (
	"github.com/juju/cmd/v3"
	"github.com/juju/gnuflag"
	"github.com/juju/retry"

	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/network/ssh"
)

const usageDebugCodeExamples = `
    juju debug-code mysql/0
    juju debug-code mysql/leader
    juju debug-code mysql/1 config-changed
    juju debug-code hello-kubecon/0 pull-site update-status
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
The command launches a tmux session that will intercept matching events and/or 
actions. It is similar to 'juju debug-hooks' but instead of dropping you into a
shell prompt, it automatically executes the hooks and/or actions and sets the
JUJU_DEBUG_AT environment variable. Charms implementing support for this
should set debug breakpoints based on the environment variable. Charms written
with the Charmed Operator Framework Ops automatically provide support for this.

Initially, the tmux session will take you to '/var/lib/juju'. As soon as a
matching event or action is fired, the hook or action is executed and the
JUJU_DEBUG_AT variable is set.

For more details on debugging charm code, see the charm SDK documentation.

Valid unit identifiers are:
  a standard unit ID, such as mysql/0 or;
  leader syntax of the form <application>/leader, such as mysql/leader.

If no event or action is specified, all events and actions will be intercepted.

See the "juju help ssh" for information about SSH related options
accepted by the debug-code command.
`

func (c *debugCodeCommand) Info() *cmd.Info {
	return jujucmd.Info(&cmd.Info{
		Name:     "debug-code",
		Args:     "<unit name> [event or action names]",
		Purpose:  "Launch a tmux session to debug hooks and/or actions.",
		Doc:      debugCodeDoc,
		Examples: usageDebugCodeExamples,
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
	if err := c.initAPIs(); err != nil {
		return err
	}
	defer c.closeAPIs()
	return c.commonRun(ctx, c.provider.getTarget(), c.hooks, c.debugAt)
}
