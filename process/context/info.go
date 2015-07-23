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

// InfoCommandInfo is the info for the proc-info command.
var InfoCommandInfo = cmdInfo{
	Name:         "process-info",
	OptionalArgs: []string{"name"},
	Summary:      "get info about a workload process (or all of them)",
	Doc: `
"info" is used while a hook is running to access a currently registered
workload process (or the list of all the unit's processes). The process
info is printed to stdout as YAML-formatted text.
`,
}

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
	c := &ProcInfoCommand{
		baseCommand: *base,
	}
	c.cmdInfo = InfoCommandInfo
	c.handleArgs = c.init
	return c, nil
}

// SetFlags implements cmd.Command.
func (c *ProcInfoCommand) SetFlags(f *gnuflag.FlagSet) {
	f.BoolVar(&c.Available, "available", false, "show unregistered processes instead")
}

func (c *ProcInfoCommand) init(args map[string]string) error {
	if len(args) > 0 {
		if c.Available {
			c.Name = args["name"]
			// Do not call c.init().
		} else if err := c.baseCommand.init(args); err != nil {
			return errors.Trace(err)
		}
	} // Otherwise we do nothing.
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
	if len(ids) == 0 {
		for _, proc := range procs {
			ids = append(ids, proc.Name)
		}
		sort.Strings(ids)
	}

	values := make(map[string]interface{})
	for k, v := range procs {
		if v == nil {
			values[k] = nil
			continue
		}
		values[k] = v
	}
	if err := dumpAll(ctx, ids, values); err != nil {
		return errors.Trace(err)
	}
	return nil
}

func (c *ProcInfoCommand) printDefinitions(ctx *cmd.Context, names ...string) error {
	definitions, err := c.defsFromCharm()
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
	values := make(map[string]interface{})
	for k, v := range definitions {
		values[k] = v
	}
	if err := dumpAll(ctx, names, values); err != nil {
		return errors.Trace(err)
	}
	//definition, ok := definitions[names[0]]
	//if !ok {
	//	return errors.NotFoundf(names[0])
	//}
	//if err := dump(ctx, ndefinition); err != nil {
	//	return errors.Trace(err)
	//}
	//for _, name := range names[1:] {
	//	definition, ok := definitions[name]
	//	if !ok {
	//		return errors.NotFoundf(name)
	//	}
	//	if err := dump(ctx, definition); err != nil {
	//		return errors.Trace(err)
	//	}
	//}
	return nil
}
