// Copyright 2012-2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package action

import (
	"github.com/juju/cmd"
	errors "github.com/juju/errors"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/names"
	"launchpad.net/gnuflag"
)

// QueueCommand shows the currently queued Actions.
type QueueCommand struct {
	ActionCommandBase
	serviceTag name.ServiceTag
	out        cmd.Output
}

const queueDoc = `
Show the currently running and pending actions for all services. A
service name can be passed to show only the actions for this service.
`

// SetFlags analyses the valid flags of the command.
func (c *QueueCommand) SetFlags(f *gnuflag.FlagSet) {
	c.out.AddFlags(f, "yaml", map[string]cmd.Formatter{
		"yaml": cmd.FormatYaml,
	})
}

// Info returns information about the command for the user.
func (c *QueueCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "queue",
		Args:    "[service name]",
		Purpose: "WIP: show currently queued actions",
		Doc:     queueDoc,
	}
}

// Init initializes the command by parsing and validating the arguments.
func (c *QueueCommand) Init(args []string) error {
	if len(args) == 1 {
		serviceName = args[0]
		if !names.IsValidService(serviceName) {
			return errors.Errorf("invalid service name %q", serviceName)
		}
		c.serviceTag = names.NewServiceTag(serviceName)
		return nil
	}
	return cmd.CheckEmpty(args[1:])
}

// Run executes the command.
func (c *QueueCommand) Run(ctx *cmd.Context) error {
	api, err := c.NewActionAPIClient()
	if err != nil {
		return err
	}
	defer api.Close()

	actionsByReceivers, err := api.ListAll(arg params.Entities)

	return c.out.Write(ctx, struct {
		Id     string
		Status string
	}{
		Id:     actionTag.Id(),
		Status: result.Status,
	})
}
