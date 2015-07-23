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
"process-info" is used while a hook is running to access a currently
registered workload process (or the list of all the unit's processes).
The process info is printed to stdout as YAML-formatted text.
`,
}

// ProcInfoCommand implements the register command.
type ProcInfoCommand struct {
	baseCommand

	output cmd.Output
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

func (c *ProcInfoCommand) SetFlags(f *gnuflag.FlagSet) {
	defaultFormat := "yaml"
	c.output.AddFlags(f, defaultFormat, map[string]cmd.Formatter{
		"yaml": cmd.FormatYaml,
		"json": cmd.FormatJson,
	})
}

func (c *ProcInfoCommand) init(args map[string]string) error {
	if len(args) == 0 {
		return nil
	}
	if err := c.baseCommand.init(args); err != nil {
		return errors.Trace(err)
	}
	return nil
}

// Run implements cmd.Command.
func (c *ProcInfoCommand) Run(ctx *cmd.Context) error {
	var ids []string
	if c.Name != "" {
		ids = append(ids, c.Name)
	}

	formatted, err := c.formatInfos(ids...)
	if err != nil {
		return errors.Trace(err)
	}
	if len(formatted) == 0 {
		fmt.Fprintln(ctx.Stderr, "<no processes registered>")
		return nil
	}

	if err := c.output.Write(ctx, formatted); err != nil {
		return errors.Trace(err)
	}
	return nil
}

func (c *ProcInfoCommand) formatInfos(ids ...string) (map[string]interface{}, error) {
	procs, err := c.registeredProcs(ids...)
	if err != nil {
		return nil, errors.Trace(err)
	}
	if len(ids) != 0 {
		if len(procs) == 0 {
			return nil, errors.NotFoundf("%v", ids)
		}
	} else {
		for _, proc := range procs {
			ids = append(ids, proc.Name)
		}
		sort.Strings(ids)
	}

	results := make(map[string]interface{}, len(ids))
	for _, id := range ids {
		proc := procs[id]
		if proc == nil {
			results[id] = "<not found>"
		} else {
			results[id] = *proc
		}
	}
	return results, nil
}
