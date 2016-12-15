// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package commands

import (
	"encoding/base64"
	"fmt"
	"sort"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	"gopkg.in/juju/charm.v6-unstable/hooks"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/api/application"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/network"
	unitdebug "github.com/juju/juju/worker/uniter/runner/debug"
)

func newDebugHooksCommand(hostDialer network.Dialer) cmd.Command {
	c := new(debugHooksCommand)
	c.setHostDialer(hostDialer)
	return modelcmd.Wrap(c)
}

// debugHooksCommand is responsible for launching a ssh shell on a given unit or machine.
type debugHooksCommand struct {
	sshCommand
	hooks []string
}

const debugHooksDoc = `
Interactively debug a hook remotely on an application unit.

See the "juju help ssh" for information about SSH related options
accepted by the debug-hooks command.
`

func (c *debugHooksCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "debug-hooks",
		Args:    "<unit name> [hook names]",
		Purpose: "Launch a tmux session to debug a hook.",
		Doc:     debugHooksDoc,
	}
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
	CharmRelations(serviceName string) ([]string, error)
}

func (c *debugHooksCommand) getServiceAPI() (charmRelationsAPI, error) {
	root, err := c.NewAPIRoot()
	if err != nil {
		return nil, errors.Trace(err)
	}
	return application.NewClient(root), nil
}

func (c *debugHooksCommand) validateHooks() error {
	if len(c.hooks) == 0 {
		return nil
	}
	service, err := names.UnitApplication(c.Target)
	if err != nil {
		return err
	}
	serviceAPI, err := c.getServiceAPI()
	if err != nil {
		return err
	}
	relations, err := serviceAPI.CharmRelations(service)
	if err != nil {
		return err
	}

	validHooks := make(map[string]bool)
	for _, hook := range hooks.UnitHooks() {
		validHooks[string(hook)] = true
	}
	for _, relation := range relations {
		for _, hook := range hooks.RelationHooks() {
			hook := fmt.Sprintf("%s-%s", relation, hook)
			validHooks[hook] = true
		}
	}
	for _, hook := range c.hooks {
		if !validHooks[hook] {
			names := make([]string, 0, len(validHooks))
			for hookName := range validHooks {
				names = append(names, hookName)
			}
			sort.Strings(names)
			logger.Infof("unknown hook %s, valid hook names: %v", hook, names)
			return errors.Errorf("unit %q does not contain hook %q", c.Target, hook)
		}
	}
	return nil
}

// Run ensures c.Target is a unit, and resolves its address,
// and connects to it via SSH to execute the debug-hooks
// script.
func (c *debugHooksCommand) Run(ctx *cmd.Context) error {
	err := c.initRun()
	if err != nil {
		return err
	}
	defer c.cleanupRun()
	err = c.validateHooks()
	if err != nil {
		return err
	}
	debugctx := unitdebug.NewHooksContext(c.Target)
	script := base64.StdEncoding.EncodeToString([]byte(unitdebug.ClientScript(debugctx, c.hooks)))
	innercmd := fmt.Sprintf(`F=$(mktemp); echo %s | base64 -d > $F; . $F`, script)
	args := []string{fmt.Sprintf("sudo /bin/bash -c '%s'", innercmd)}
	c.Args = args
	return c.sshCommand.Run(ctx)
}
