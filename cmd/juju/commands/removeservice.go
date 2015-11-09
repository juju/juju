// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package commands

import (
	"fmt"

	"github.com/juju/cmd"
	"github.com/juju/names"

	"github.com/juju/juju/cmd/envcmd"
	"github.com/juju/juju/cmd/juju/block"
)

func newRemoveServiceCommand() cmd.Command {
	return envcmd.Wrap(&removeServiceCommand{})
}

// removeServiceCommand causes an existing service to be destroyed.
type removeServiceCommand struct {
	envcmd.EnvCommandBase
	ServiceName string
}

const removeServiceDoc = `
Removing a service will remove all its units and relations.

If this is the only service running, the machine on which
the service is hosted will also be destroyed, if possible.
The machine will be destroyed if:
- it is not a state server
- it is not hosting any Juju managed containers
`

func (c *removeServiceCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "remove-service",
		Args:    "<service>",
		Purpose: "remove a service from the environment",
		Doc:     removeServiceDoc,
		Aliases: []string{"destroy-service"},
	}
}

func (c *removeServiceCommand) Init(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("no service specified")
	}
	if !names.IsValidService(args[0]) {
		return fmt.Errorf("invalid service name %q", args[0])
	}
	c.ServiceName, args = args[0], args[1:]
	return cmd.CheckEmpty(args)
}

func (c *removeServiceCommand) Run(_ *cmd.Context) error {
	client, err := c.NewAPIClient()
	if err != nil {
		return err
	}
	defer client.Close()
	return block.ProcessBlockedError(client.ServiceDestroy(c.ServiceName), block.BlockRemove)
}
