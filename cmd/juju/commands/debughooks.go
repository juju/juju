// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package commands

import (
	"encoding/base64"
	"fmt"
	"strings"

	"github.com/juju/charm/v9/hooks"
	"github.com/juju/cmd"
	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"github.com/juju/names/v4"

	"github.com/juju/juju/api/action"
	"github.com/juju/juju/api/application"
	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/network/ssh"
	unitdebug "github.com/juju/juju/worker/uniter/runner/debug"
)

func newDebugHooksCommand(hostChecker ssh.ReachableChecker) cmd.Command {
	c := new(debugHooksCommand)
	c.hostChecker = hostChecker
	return modelcmd.Wrap(c)
}

// debugHooksCommand is responsible for launching a ssh shell on a given unit or machine.
type debugHooksCommand struct {
	sshCommand
	hooks []string

	actionsAPI
	charmRelationsAPI
}

const debugHooksDoc = `
Interactively debug hooks or actions remotely on an application unit.

Valid unit identifiers are:
  a standard unit ID, such as mysql/0 or;
  leader syntax of the form <application>/leader, such as mysql/leader.

See the "juju help ssh" for information about SSH related options
accepted by the debug-hooks command.
`

func (c *debugHooksCommand) Info() *cmd.Info {
	return jujucmd.Info(&cmd.Info{
		Name:    "debug-hooks",
		Args:    "<unit name> [hook or action names]",
		Purpose: "Launch a tmux session to debug hooks and/or actions.",
		Doc:     debugHooksDoc,
		Aliases: []string{"debug-hook"},
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

type charmRelationsAPI interface {
	CharmRelations(applicationName string) ([]string, error)
	Close() error
}

type actionsAPI interface {
	ApplicationCharmActions(string) (map[string]action.ActionSpec, error)
	Close() error
}

func (c *debugHooksCommand) initAPIs() (err error) {
	if c.actionsAPI != nil && c.charmRelationsAPI != nil {
		return nil
	}

	root, err := c.NewAPIRoot()
	if err != nil {
		return errors.Trace(err)
	}

	if c.actionsAPI == nil {
		c.actionsAPI = action.NewClient(root)
	}
	if c.charmRelationsAPI == nil {
		c.charmRelationsAPI = application.NewClient(root)
	}
	return nil
}

func (c *debugHooksCommand) closeAPIs() {
	if c.actionsAPI != nil {
		_ = c.actionsAPI.Close()
		c.actionsAPI = nil
	}
	if c.charmRelationsAPI != nil {
		_ = c.charmRelationsAPI.Close()
		c.charmRelationsAPI = nil
	}
}

func (c *debugHooksCommand) validateHooksOrActions() error {
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

	// Get a set of valid hooks.
	validHooks, err := c.getValidHooks(appName)
	if err != nil {
		return err
	}

	// Get a set of valid actions.
	validActions, err := c.getValidActions(appName)
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

func (c *debugHooksCommand) getValidActions(appName string) (set.Strings, error) {
	allActions, err := c.actionsAPI.ApplicationCharmActions(appName)
	if err != nil {
		return nil, err
	}

	validActions := set.NewStrings()
	for name := range allActions {
		validActions.Add(name)
	}
	return validActions, nil
}

func (c *debugHooksCommand) getValidHooks(appName string) (set.Strings, error) {
	relations, err := c.charmRelationsAPI.CharmRelations(appName)
	if err != nil {
		return nil, err
	}

	validHooks := set.NewStrings()
	for _, hook := range hooks.UnitHooks() {
		validHooks.Add(string(hook))
	}
	for _, relation := range relations {
		for _, hook := range hooks.RelationHooks() {
			hook := fmt.Sprintf("%s-%s", relation, hook)
			validHooks.Add(hook)
		}
	}
	return validHooks, nil
}

func (c *debugHooksCommand) decideEntryPoint(ctx *cmd.Context) string {
	if c.modelType == model.CAAS {
		c.provider.setArgs([]string{"which", "sudo"})
		if err := c.sshCommand.Run(ctx); err != nil {
			return "/bin/bash -c '%s'"
		}
	}
	return "sudo /bin/bash -c '%s'"
}

// commonRun is shared between debugHooks and debugCode
func (c *debugHooksCommand) commonRun(
	ctx *cmd.Context,
	target string,
	hooks []string,
	debugAt string,
) (err error) {
	err = c.validateHooksOrActions()
	if err != nil {
		return err
	}

	// If the unit/leader syntax is used, we first need to resolve it into
	// the unit name that corresponds to the current leader.
	resolvedTargetName, err := maybeResolveLeaderUnit(func() (StatusAPI, error) {
		return c.ModelCommandBase.NewAPIClient()
	}, target)
	if err != nil {
		return errors.Trace(err)
	}

	debugctx := unitdebug.NewHooksContext(resolvedTargetName)
	clientScript := unitdebug.ClientScript(debugctx, hooks, debugAt)
	b64Script := base64.StdEncoding.EncodeToString([]byte(clientScript))
	innercmd := fmt.Sprintf(`F=$(mktemp); echo %s | base64 -d > $F; . $F`, b64Script)
	args := []string{fmt.Sprintf(c.decideEntryPoint(ctx), innercmd)}
	c.provider.setArgs(args)
	return c.sshCommand.Run(ctx)
}

// Run ensures Target is a unit, and resolves its address,
// and connects to it via SSH to execute the debug-hooks
// script.
func (c *debugHooksCommand) Run(ctx *cmd.Context) error {
	if err := c.initAPIs(); err != nil {
		return err
	}
	defer c.closeAPIs()
	return c.commonRun(ctx, c.provider.getTarget(), c.hooks, "")
}
