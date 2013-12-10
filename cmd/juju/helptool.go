// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"fmt"

	"launchpad.net/gnuflag"

	"launchpad.net/juju-core/charm"
	"launchpad.net/juju-core/cmd"
	"launchpad.net/juju-core/worker/uniter/jujuc"
)

// dummyHookContext implements jujuc.Context,
// as expected by jujuc.NewCommand.
type dummyHookContext struct{}

func (dummyHookContext) UnitName() string {
	return ""
}
func (dummyHookContext) PublicAddress() (string, bool) {
	return "", false
}
func (dummyHookContext) PrivateAddress() (string, bool) {
	return "", false
}
func (dummyHookContext) OpenPort(protocol string, port int) error {
	return nil
}
func (dummyHookContext) ClosePort(protocol string, port int) error {
	return nil
}
func (dummyHookContext) ConfigSettings() (charm.Settings, error) {
	return charm.NewConfig().DefaultSettings(), nil
}
func (dummyHookContext) HookRelation() (jujuc.ContextRelation, bool) {
	return nil, false
}
func (dummyHookContext) RemoteUnitName() (string, bool) {
	return "", false
}
func (dummyHookContext) Relation(id int) (jujuc.ContextRelation, bool) {
	return nil, false
}
func (dummyHookContext) RelationIds() []int {
	return []int{}
}

func (dummyHookContext) OwnerTag() string {
	return ""
}

type HelpToolCommand struct {
	cmd.CommandBase
	tool string
}

func (t *HelpToolCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "help-tool",
		Args:    "[tool]",
		Purpose: "show help on a juju charm tool",
	}
}

func (t *HelpToolCommand) Init(args []string) error {
	tool, err := cmd.ZeroOrOneArgs(args)
	if err == nil {
		t.tool = tool
	}
	return err
}

func (c *HelpToolCommand) Run(ctx *cmd.Context) error {
	var hookctx dummyHookContext
	if c.tool == "" {
		// Ripped from SuperCommand. We could Run() a SuperCommand
		// with "help commands", but then the implicit "help" command
		// shows up.
		names := jujuc.CommandNames()
		cmds := make([]cmd.Command, 0, len(names))
		longest := 0
		for _, name := range names {
			if c, err := jujuc.NewCommand(hookctx, name); err == nil {
				if len(name) > longest {
					longest = len(name)
				}
				cmds = append(cmds, c)
			}
		}
		for _, c := range cmds {
			info := c.Info()
			fmt.Fprintf(ctx.Stdout, "%-*s  %s\n", longest, info.Name, info.Purpose)
		}
	} else {
		c, err := jujuc.NewCommand(hookctx, c.tool)
		if err != nil {
			return err
		}
		info := c.Info()
		f := gnuflag.NewFlagSet(info.Name, gnuflag.ContinueOnError)
		c.SetFlags(f)
		ctx.Stdout.Write(info.Help(f))
	}
	return nil
}
