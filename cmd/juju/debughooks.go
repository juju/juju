// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"encoding/base64"
	"errors"
	"fmt"
	"sort"

	"launchpad.net/juju-core/charm/hooks"
	"launchpad.net/juju-core/cmd"
	"launchpad.net/juju-core/names"
	unitdebug "launchpad.net/juju-core/worker/uniter/debug"
)

// DebugHooksCommand is responsible for launching a ssh shell on a given unit or machine.
type DebugHooksCommand struct {
	SSHCommand
	hooks []string
}

const debugHooksDoc = `
Interactively debug a hook remotely on a service unit.
`

func (c *DebugHooksCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "debug-hooks",
		Args:    "<unit name> [hook names]",
		Purpose: "launch a tmux session to debug a hook",
		Doc:     debugHooksDoc,
	}
}

func (c *DebugHooksCommand) Init(args []string) error {
	if len(args) < 1 {
		return errors.New("no unit name specified")
	}
	c.Target = args[0]
	if !names.IsUnit(c.Target) {
		return fmt.Errorf("%q is not a valid unit name", c.Target)
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

func (c *DebugHooksCommand) validateHooks() error {
	if len(c.hooks) == 0 {
		return nil
	}
	service := names.UnitService(c.Target)
	relations, err := c.apiClient.ServiceCharmRelations(service)
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
			for hookName, _ := range validHooks {
				names = append(names, hookName)
			}
			sort.Strings(names)
			logger.Infof("unknown hook %s, valid hook names: %v", hook, names)
			return fmt.Errorf("unit %q does not contain hook %q", c.Target, hook)
		}
	}
	return nil
}

// Run ensures c.Target is a unit, and resolves its address,
// and connects to it via SSH to execute the debug-hooks
// script.
func (c *DebugHooksCommand) Run(ctx *cmd.Context) error {
	var err error
	c.apiClient, err = c.initAPIClient()
	if err != nil {
		return err
	}
	defer c.apiClient.Close()
	err = c.validateHooks()
	if err != nil {
		return err
	}
	debugctx := unitdebug.NewHooksContext(c.Target)
	script := base64.StdEncoding.EncodeToString([]byte(unitdebug.ClientScript(debugctx, c.hooks)))
	innercmd := fmt.Sprintf(`F=$(mktemp); echo %s | base64 -d > $F; . $F`, script)
	args := []string{fmt.Sprintf("sudo /bin/bash -c '%s'", innercmd)}
	c.Args = args
	return c.SSHCommand.Run(ctx)
}
