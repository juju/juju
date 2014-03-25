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
	"launchpad.net/juju-core/state/api/params"
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

// getRelationNames1dot16 gets the list of relation hooks directly from the
// database, in a fashion compatible with the API server in Juju 1.16 (which
// doesn't have the ServiceCharmRelations API). This function can be removed
// when we no longer maintain compatibility with 1.16
func (c *DebugHooksCommand) getRelationNames1dot16() ([]string, error) {
	err := c.ensureRawConn()
	if err != nil {
		return nil, err
	}
	unit, err := c.rawConn.State.Unit(c.Target)
	if err != nil {
		return nil, err
	}
	service, err := unit.Service()
	if err != nil {
		return nil, err
	}
	endpoints, err := service.Endpoints()
	if err != nil {
		return nil, err
	}
	relations := make([]string, len(endpoints))
	for i, endpoint := range endpoints {
		relations[i] = endpoint.Relation.Name
	}
	return relations, nil
}

func (c *DebugHooksCommand) getRelationNames(serviceName string) ([]string, error) {
	relations, err := c.apiClient.ServiceCharmRelations(serviceName)
	if params.IsCodeNotImplemented(err) {
		logger.Infof("API server does not support Client.ServiceCharmRelations falling back to 1.16 compatibility mode (direct DB access)")
		return c.getRelationNames1dot16()
	}
	if err != nil {
		return nil, err
	}
	return relations, err
}

func (c *DebugHooksCommand) validateHooks() error {
	if len(c.hooks) == 0 {
		return nil
	}
	service := names.UnitService(c.Target)
	relations, err := c.getRelationNames(service)
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
