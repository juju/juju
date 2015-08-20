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

// InfoCommandInfo is the info for the workload-info command.
var InfoCommandInfo = cmdInfo{
	Name:         "workload-info",
	OptionalArgs: []string{idArg},
	Summary:      "get info about a workload (or all of them)",
	Doc: `
"workload-info" is used while a hook is running to access a currently
tracked workload (or the list of all the unit's workloads).
The workload info is printed to stdout as YAML-formatted text.
`,
}

// WorkloadInfoCommand implements the register command.
type WorkloadInfoCommand struct {
	baseCommand

	output cmd.Output
}

// NewWorkloadInfoCommand returns a new WorkloadInfoCommand.
func NewWorkloadInfoCommand(ctx HookContext) (*WorkloadInfoCommand, error) {
	base, err := newCommand(ctx)
	if err != nil {
		return nil, errors.Trace(err)
	}
	c := &WorkloadInfoCommand{
		baseCommand: *base,
	}
	c.cmdInfo = InfoCommandInfo
	c.handleArgs = c.init
	return c, nil
}

func (c *WorkloadInfoCommand) SetFlags(f *gnuflag.FlagSet) {
	defaultFormat := "yaml"
	c.output.AddFlags(f, defaultFormat, map[string]cmd.Formatter{
		"yaml": cmd.FormatYaml,
		"json": cmd.FormatJson,
	})
}

func (c *WorkloadInfoCommand) init(args map[string]string) error {
	if len(args) == 0 {
		return nil
	}
	if err := c.baseCommand.init(args); err != nil {
		return errors.Trace(err)
	}
	return nil
}

// Run implements cmd.Command.
func (c *WorkloadInfoCommand) Run(ctx *cmd.Context) error {
	var ids []string
	if c.ID != "" {
		id, err := c.findID()
		if errors.IsNotFound(err) {
			id = c.ID
		} else if err != nil {
			return errors.Trace(err)
		}
		ids = append(ids, id)
	}

	formatted, err := c.formatInfos(ids...)
	if err != nil {
		return errors.Trace(err)
	}
	if len(formatted) == 0 {
		fmt.Fprintln(ctx.Stderr, "<no workloads tracked>")
		return nil
	}

	if err := c.output.Write(ctx, formatted); err != nil {
		return errors.Trace(err)
	}
	return nil
}

func (c *WorkloadInfoCommand) formatInfos(ids ...string) (map[string]interface{}, error) {
	workloads, err := c.trackedWorkloads(ids...)
	if err != nil {
		return nil, errors.Trace(err)
	}
	if len(ids) != 0 {
		if len(workloads) == 0 {
			return nil, errors.NotFoundf("%v", ids)
		}
	} else {
		for _, wl := range workloads {
			ids = append(ids, wl.ID())
		}
		sort.Strings(ids)
	}

	results := make(map[string]interface{}, len(ids))
	for _, id := range ids {
		wl := workloads[id]
		if wl == nil {
			results[id] = "<not found>"
		} else {
			results[id] = *wl
		}
	}
	return results, nil
}
