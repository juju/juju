// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package commands

import (
	"encoding/base64"
	"fmt"

	"github.com/juju/charm/v7/hooks"
	"github.com/juju/cmd"
	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"github.com/juju/names/v4"

	"github.com/juju/juju/api/action"
	"github.com/juju/juju/api/application"
	"github.com/juju/juju/apiserver/params"
	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/network/ssh"
	unitdebug "github.com/juju/juju/worker/uniter/runner/debug"
)

func newDebugHooksCommand(hostChecker ssh.ReachableChecker) cmd.Command {
	c := new(debugHooksCommand)
	c.getActionAPI = c.newActionsAPI
	c.setHostChecker(hostChecker)
	return modelcmd.Wrap(c)
}

// debugHooksCommand is responsible for launching a ssh shell on a given unit or machine.
type debugHooksCommand struct {
	sshCommand
	hooks []string

	getActionAPI func() (ActionsAPI, error)
}

const debugHooksDoc = `
Interactively debug hooks or actions remotely on an application unit.

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
	c.Target = args[0]
	if !names.IsValidUnit(c.Target) {
		return errors.Errorf("%q is not a valid unit name", c.Target)
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
}

type ActionsAPI interface {
	ApplicationCharmActions(params.Entity) (map[string]params.ActionSpec, error)
}

func (c *debugHooksCommand) getApplicationAPI() (charmRelationsAPI, error) {
	root, err := c.NewAPIRoot()
	if err != nil {
		return nil, errors.Trace(err)
	}
	return application.NewClient(root), nil
}

func (c *debugHooksCommand) newActionsAPI() (ActionsAPI, error) {
	root, err := c.NewAPIRoot()
	if err != nil {
		return nil, err
	}
	return action.NewClient(root), nil
}

func (c *debugHooksCommand) validateHooksOrActions() error {
	if len(c.hooks) == 0 {
		return nil
	}
	appName, err := names.UnitApplication(c.Target)
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
				c.Target,
				hook,
				validActions.SortedValues(),
				validHooks.SortedValues(),
			)
		}
	}
	return nil
}

func (c *debugHooksCommand) getValidActions(appName string) (set.Strings, error) {
	appTag := names.NewApplicationTag(appName)
	actionAPI, err := c.getActionAPI()
	if err != nil {
		return nil, err
	}

	allActions, err := actionAPI.ApplicationCharmActions(params.Entity{Tag: appTag.String()})
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
	applicationAPI, err := c.getApplicationAPI()
	if err != nil {
		return nil, err
	}
	relations, err := applicationAPI.CharmRelations(appName)
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

// commonRun is shared between debugHooks and debugCode
func (c *debugHooksCommand) commonRun(
	ctx *cmd.Context,
	target string,
	hooks []string,
	debugAt string,
) error {

	err := c.initRun()
	if err != nil {
		return err
	}
	defer c.cleanupRun()
	err = c.validateHooksOrActions()
	if err != nil {
		return err
	}
	debugctx := unitdebug.NewHooksContext(target)
	clientScript := unitdebug.ClientScript(debugctx, hooks, debugAt)
	b64Script := base64.StdEncoding.EncodeToString([]byte(clientScript))
	innercmd := fmt.Sprintf(`F=$(mktemp); echo %s | base64 -d > $F; . $F`, b64Script)
	args := []string{fmt.Sprintf("sudo /bin/bash -c '%s'", innercmd)}
	c.Args = args
	return c.sshCommand.Run(ctx)
}

// Run ensures c.Target is a unit, and resolves its address,
// and connects to it via SSH to execute the debug-hooks
// script.
func (c *debugHooksCommand) Run(ctx *cmd.Context) error {
	return c.commonRun(ctx, c.Target, c.hooks, "")
}
