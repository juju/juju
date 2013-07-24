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

func (_ dummyHookContext) UnitName() string {
	return ""
}
func (_ dummyHookContext) PublicAddress() (string, bool) {
	return "", false
}
func (_ dummyHookContext) PrivateAddress() (string, bool) {
	return "", false
}
func (_ dummyHookContext) OpenPort(protocol string, port int) error {
	return nil
}
func (_ dummyHookContext) ClosePort(protocol string, port int) error {
	return nil
}
func (_ dummyHookContext) ConfigSettings() (charm.Settings, error) {
	return charm.NewConfig().DefaultSettings(), nil
}
func (_ dummyHookContext) HookRelation() (jujuc.ContextRelation, bool) {
	return nil, false
}
func (_ dummyHookContext) RemoteUnitName() (string, bool) {
	return "", false
}
func (_ dummyHookContext) Relation(id int) (jujuc.ContextRelation, bool) {
	return nil, false
}
func (_ dummyHookContext) RelationIds() []int {
	return []int{}
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
		cmds := make([]cmd.Command, len(names))
		longest := 0
		for i, name := range names {
			if c, err := jujuc.NewCommand(hookctx, name); err == nil {
				if len(name) > longest {
					longest = len(name)
				}
				cmds[i] = c
			}
		}
		const lineFormat = "%-*s  %s\n"
		const outputFormat = "%s"
		for i, name := range names {
			var purpose string
			if cmds[i] != nil {
				purpose = cmds[i].Info().Purpose
			}
			fmt.Fprintf(ctx.Stdout, lineFormat, longest, name, purpose)
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
