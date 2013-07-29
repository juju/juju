// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"encoding/base64"
	"errors"
	"fmt"
	"os/exec"

	"launchpad.net/juju-core/charm/hooks"
	"launchpad.net/juju-core/cmd"
	"launchpad.net/juju-core/juju"
	"launchpad.net/juju-core/state"
	unitdebug "launchpad.net/juju-core/worker/uniter/debug"
)

// DebugHooksCommand is responsible for launching a ssh shell on a given unit or machine.
type DebugHooksCommand struct {
	SSHCommon
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

func (c *DebugHooksCommand) validateHooks(unit *state.Unit) error {
	if len(c.hooks) == 0 {
		return nil
	}
	service, err := unit.Service()
	if err != nil {
		return err
	}
	eps, err := service.Endpoints()
	if err != nil {
		return err
	}
	validHooks := make(map[string]bool)
	for _, hook := range hooks.UnitHooks() {
		validHooks[string(hook)] = true
	}
	for _, ep := range eps {
		for _, hook := range hooks.RelationHooks() {
			hook := fmt.Sprintf("%s-%s", ep.Relation.Name, hook)
			validHooks[hook] = true
		}
	}
	for _, hook := range c.hooks {
		if !validHooks[hook] {
			return fmt.Errorf("unit %q does not contain hook %q", unit.Name(), hook)
		}
	}
	return nil
}

// Run ensures c.Target is a unit, and resolves its address,
// and connects to it via SSH to execute the debug-hooks
// script.
func (c *DebugHooksCommand) Run(ctx *cmd.Context) error {
	var err error
	c.Conn, err = juju.NewConnFromName(c.EnvName)
	if err != nil {
		return err
	}
	defer c.Close()
	unit, err := c.Conn.State.Unit(c.Target)
	if err != nil {
		return err
	}
	err = c.validateHooks(unit)
	if err != nil {
		return err
	}
	host, err := c.hostFromTarget(c.Target)
	if err != nil {
		return err
	}

	debugctx := unitdebug.NewDebugHooksContext(c.Target)
	args := []string{"-l", "ubuntu", "-t", "-o", "StrictHostKeyChecking no", "-o", "PasswordAuthentication no", host, "--"}
	script := base64.StdEncoding.EncodeToString([]byte(debugctx.ClientScript(c.hooks)))
	args = append(args, fmt.Sprintf("sudo /bin/bash -c '%s'", fmt.Sprintf(`F=$(mktemp); echo %s | base64 -d > $F; . $F`, script)))
	cmd := exec.Command("ssh", args...)
	cmd.Stdin = ctx.Stdin
	cmd.Stdout = ctx.Stdout
	cmd.Stderr = ctx.Stderr
	c.Close()
	return cmd.Run()
}
