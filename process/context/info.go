// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package context

import (
	"fmt"
	"sort"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	"launchpad.net/gnuflag"
)

const infoDoc = `
"info" is used while a hook is running to access a currently registered
workload process (or the list of all the unit's processes). The process
info is printed to stdout as YAML-formatted text.
`

// ProcInfoCommand implements the register command.
type ProcInfoCommand struct {
	baseCommand

	// Available indicates that only unregistered process definitions
	// from the charm metadata should be shown.
	Available bool
}

// NewProcInfoCommand returns a new ProcInfoCommand.
func NewProcInfoCommand(ctx HookContext) (*ProcInfoCommand, error) {
	base, err := newCommand(ctx)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return &ProcInfoCommand{
		baseCommand: *base,
	}, nil
}

// Info implements cmd.Command.
func (c *ProcInfoCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "info",
		Args:    "[<name>]",
		Purpose: "get info about a workload process (or all of them)",
		Doc:     infoDoc,
	}
}

// SetFlags implements cmd.Command.
func (c *ProcInfoCommand) SetFlags(f *gnuflag.FlagSet) {
	f.BoolVar(&c.Available, "available", false, "show unregistered processes instead")
}

// Init implements cmd.Command.
func (c *ProcInfoCommand) Init(args []string) error {
	if len(args) > 1 {
		return errors.Errorf("expected <name> (or nothing), got: %v", args)
	}
	if len(args) > 0 {
		if c.Available {
			c.Name = args[0]
			// Do not call c.init().
		} else if err := c.init(args[0]); err != nil {
			return errors.Trace(err)
		}
	} // Otherwise we do *not* call c.init().
	return nil
}

// Run implements cmd.Command.
func (c *ProcInfoCommand) Run(ctx *cmd.Context) error {
	var ids []string
	if c.Name != "" {
		ids = append(ids, c.Name)
	}

	if c.Available {
		if err := c.printDefinitions(ctx, ids...); err != nil {
			return errors.Trace(err)
		}
	} else {
		if err := c.printInfos(ctx, ids...); err != nil {
			return errors.Trace(err)
		}
	}
	return nil
}

func (c *ProcInfoCommand) printInfos(ctx *cmd.Context, ids ...string) error {
	procs, err := c.registeredProcs(ids...)
	if err != nil {
		return errors.Trace(err)
	}
	if len(procs) == 0 {
		if len(ids) != 0 {
			return errors.NotFoundf("%v", ids)
		}
		fmt.Fprintln(ctx.Stderr, " [no processes registered]")
		return nil
	}
	// TODO(ericsnow) Sort if len(ids) == 0.
	var values []interface{}
	for _, v := range procs {
		values = append(values, v)
	}
	if err := dumpAll(ctx, values...); err != nil {
		return errors.Trace(err)
	}
	return nil
}

func (c *ProcInfoCommand) printDefinitions(ctx *cmd.Context, names ...string) error {
	definitions, err := c.defsFromCharm(ctx)
	if err != nil {
		return errors.Trace(err)
	}
	if len(names) == 0 {
		if len(definitions) == 0 {
			fmt.Fprintln(ctx.Stderr, " [no processes defined in charm]")
			return nil
		}
		for _, def := range definitions {
			names = append(names, def.Name)
		}
		sort.Strings(names)
	}

	// Now print them out.
	definition, ok := definitions[names[0]]
	if !ok {
		return errors.NotFoundf(names[0])
	}
	if err := dump(ctx, definition); err != nil {
		return errors.Trace(err)
	}
	for _, name := range names[1:] {
		definition, ok := definitions[name]
		if !ok {
			return errors.NotFoundf(name)
		}
		if err := dump(ctx, definition); err != nil {
			return errors.Trace(err)
		}
	}
	return nil
}
