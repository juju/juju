// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"fmt"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/names"

	apiservice "github.com/juju/juju/api/service"
	"github.com/juju/juju/cmd/juju/block"
	"github.com/juju/juju/cmd/modelcmd"
)

// NewRemoveServiceCommand returns a command which removes a service.
func NewRemoveServiceCommand() cmd.Command {
	return modelcmd.Wrap(&removeServiceCommand{})
}

// removeServiceCommand causes an existing service to be destroyed.
type removeServiceCommand struct {
	modelcmd.ModelCommandBase
	ServiceName string
}

const removeServiceDoc = `
Removing a service will remove all its units and relations.

If this is the only service running, the machine on which
the service is hosted will also be destroyed, if possible.
The machine will be destroyed if:
- it is not a controller
- it is not hosting any Juju managed containers
`

func (c *removeServiceCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "remove-service",
		Args:    "<service>",
		Purpose: "remove a service from the model",
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

type ServiceRemoveAPI interface {
	Close() error
	ServiceDestroy(serviceName string) error
	DestroyServiceUnits(unitNames ...string) error
}

func (c *removeServiceCommand) getAPI() (ServiceRemoveAPI, error) {
	root, err := c.NewAPIRoot()
	if err != nil {
		return nil, errors.Trace(err)
	}
	return apiservice.NewClient(root), nil
}

func (c *removeServiceCommand) Run(_ *cmd.Context) error {
	client, err := c.getAPI()
	if err != nil {
		return err
	}
	defer client.Close()
	return block.ProcessBlockedError(client.ServiceDestroy(c.ServiceName), block.BlockRemove)
}
