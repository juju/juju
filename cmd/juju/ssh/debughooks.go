// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package ssh

import (
	"context"
	"encoding/base64"
	"fmt"
	"strings"

	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"github.com/juju/names/v6"
	"github.com/juju/retry"

	"github.com/juju/juju/api/client/application"
	"github.com/juju/juju/api/client/charms"
	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/internal/charm"
	"github.com/juju/juju/internal/charm/hooks"
	"github.com/juju/juju/internal/cmd"
	"github.com/juju/juju/internal/network/ssh"
	unitdebug "github.com/juju/juju/internal/worker/uniter/runner/debug"
)

const usageDebugHooksExamples = `
Debug all hooks and actions of unit '0':

    juju debug-hooks mysql/0

Debug all hooks and actions of the leader:

    juju debug-hooks mysql/leader

Debug the 'config-changed' hook of unit '1':

    juju debug-hooks mysql/1 config-changed

Debug the 'pull-site' action and 'update-status' hook of unit '0':

    juju debug-hooks hello-kubecon/0 pull-site update-status
`

func NewDebugHooksCommand(hostChecker ssh.ReachableChecker, retryStrategy retry.CallArgs, publicKeyRetryStrategy retry.CallArgs) cmd.Command {
	c := new(debugHooksCommand)
	c.hostChecker = hostChecker
	c.retryStrategy = retryStrategy
	c.publicKeyRetryStrategy = publicKeyRetryStrategy
	return modelcmd.Wrap(c)
}

// debugHooksCommand is responsible for launching a ssh shell on a given unit or machine.
type debugHooksCommand struct {
	sshCommand
	hooks []string
}

const debugHooksDoc = `
The command launches a tmux session that will intercept matching hooks and/or
actions.

Initially, the tmux session will take you to '/var/lib/juju' or '/home/ubuntu'.
As soon as a matching hook or action is fired, the tmux session will
automatically navigate you to '/var/lib/juju/agents/<unit-id>/charm' with a
properly configured environment. Unlike the 'juju debug-code' command,
the fired hooks and/or actions are not executed directly; instead, the user
needs to manually run the dispatch script inside the charm's directory.

Valid unit identifiers are:
  a standard unit ID, such as mysql/0 or;
  leader syntax of the form <application>/leader, such as mysql/leader.

If no hook or action is specified, all hooks and actions will be intercepted.

See the "juju help ssh" for information about SSH related options
accepted by the debug-hooks command.
`

func (c *debugHooksCommand) Info() *cmd.Info {
	return jujucmd.Info(&cmd.Info{
		Name:     "debug-hooks",
		Args:     "<unit name> [hook or action names]",
		Purpose:  "Launch a tmux session to debug hooks and/or actions.",
		Doc:      debugHooksDoc,
		Examples: usageDebugHooksExamples,
		Aliases:  []string{"debug-hook"},
		SeeAlso: []string{
			"ssh",
			"debug-code",
		},
	})
}

func (c *debugHooksCommand) Init(args []string) error {
	if len(args) < 1 {
		return errors.Errorf("no unit name specified")
	}
	if err := c.sshCommand.Init(args); err != nil {
		return err
	}

	c.provider.setTarget(args[0])
	if target := c.provider.getTarget(); !(names.IsValidUnit(target) || strings.HasSuffix(target, "/leader")) {
		return errors.Errorf("%q is not a valid unit name", c.provider.getTarget())
	}

	// If any of the hooks is "*", then debug all hooks.
	c.hooks = append([]string{}, args[1:]...)
	for _, h := range c.hooks {
		if h == "*" {
			c.hooks = nil
			break
		}
	}
	return nil
}

func (c *debugHooksCommand) initAPIs(ctx context.Context) (err error) {
	defer func() {
		c.provider.setLeaderAPI(ctx, c.applicationAPI)
	}()

	if c.charmAPI != nil && c.applicationAPI != nil {
		return nil
	}

	root, err := c.NewAPIRoot(ctx)
	if err != nil {
		return errors.Trace(err)
	}

	if c.applicationAPI == nil {
		c.applicationAPI = application.NewClient(root)
	}
	if c.charmAPI == nil {
		c.charmAPI = charms.NewClient(root)
	}
	return nil
}

func (c *debugHooksCommand) closeAPIs() {
	if c.applicationAPI != nil {
		_ = c.applicationAPI.Close()
		c.applicationAPI = nil
	}
	if c.charmAPI != nil {
		_ = c.charmAPI.Close()
		c.charmAPI = nil
	}
}

func (c *debugHooksCommand) validateHooksOrActions(ctx context.Context) error {
	if len(c.hooks) == 0 {
		return nil
	}

	var (
		appName string
		err     error
	)

	// If the unit/leader syntax is used, we need to manually extract the
	// application name from the target parameter.
	target := c.provider.getTarget()
	if strings.HasSuffix(target, "/leader") {
		appName = strings.TrimSuffix(target, "/leader")
	} else {
		appName, err = names.UnitApplication(target)
	}

	if err != nil {
		return err
	}

	curl, _, err := c.applicationAPI.GetCharmURLOrigin(ctx, appName)
	if err != nil {
		return err
	}

	charmInfo, err := c.charmAPI.CharmInfo(ctx, curl.String())
	if err != nil {
		return err
	}

	// Get a set of valid hooks.
	validHooks, err := c.getValidHooks(charmInfo.Meta)
	if err != nil {
		return err
	}

	// Get a set of valid actions.
	validActions, err := c.getValidActions(charmInfo.Actions)
	if err != nil {
		return err
	}

	// Is passed argument a valid hook or action name?
	// If not valid, err out.
	allValid := validHooks.Union(validActions)
	for _, hook := range c.hooks {
		if !allValid.Contains(hook) {
			return errors.Errorf("unit %q contains neither hook nor action %q, valid actions are %v and valid hooks are %v",
				c.provider.getTarget(),
				hook,
				validActions.SortedValues(),
				validHooks.SortedValues(),
			)
		}
	}
	return nil
}

func (c *debugHooksCommand) getValidActions(actions *charm.Actions) (set.Strings, error) {
	validActions := set.NewStrings()
	for name := range actions.ActionSpecs {
		validActions.Add(name)
	}
	return validActions, nil
}

func (c *debugHooksCommand) getValidHooks(meta *charm.Meta) (set.Strings, error) {
	validHooks := set.NewStrings()
	for _, hook := range hooks.RelationHooks() {
		hook := fmt.Sprintf("juju-info-%s", hook)
		validHooks.Add(hook)
	}
	return validHooks.Union(meta.Hooks()), nil
}

func (c *debugHooksCommand) decideEntryPoint(ctx *cmd.Context) string {
	if c.modelType == model.CAAS {
		return "exec /bin/bash -c '%s'"
	}
	return "exec sudo /bin/bash -c '%s'"
}

// commonRun is shared between debugHooks and debugCode
func (c *debugHooksCommand) commonRun(
	ctx *cmd.Context,
	target string,
	hooks []string,
	debugAt string,
) (err error) {
	err = c.validateHooksOrActions(ctx)
	if err != nil {
		return err
	}

	// If the unit/leader syntax is used, we first need to resolve it into
	// the unit name that corresponds to the current leader.
	resolvedTargetName, err := c.provider.maybeResolveLeaderUnit(ctx, target)
	if err != nil {
		return errors.Trace(err)
	}

	debugctx := unitdebug.NewHooksContext(resolvedTargetName)
	clientScript := unitdebug.ClientScript(debugctx, hooks, debugAt)
	b64Script := base64.StdEncoding.EncodeToString([]byte(clientScript))
	innercmd := fmt.Sprintf(`F=$(mktemp); echo %s | base64 -d > $F; chmod +x $F; exec $F`, b64Script)
	args := []string{fmt.Sprintf(c.decideEntryPoint(ctx), innercmd)}
	c.provider.setArgs(args)
	return c.sshCommand.Run(ctx)
}

// Run ensures Target is a unit, and resolves its address,
// and connects to it via SSH to execute the debug-hooks
// script.
func (c *debugHooksCommand) Run(ctx *cmd.Context) error {
	if err := c.initAPIs(ctx); err != nil {
		return err
	}
	defer c.closeAPIs()
	return c.commonRun(ctx, c.provider.getTarget(), c.hooks, "")
}
