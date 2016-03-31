// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"fmt"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/names"
	"github.com/juju/romulus/api/budget"
	wireformat "github.com/juju/romulus/wireformat/budget"
	"gopkg.in/juju/charm.v6-unstable"
	"gopkg.in/macaroon-bakery.v1/httpbakery"

	"github.com/juju/juju/api/charms"
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

type ServiceAPI interface {
	Close() error
	Destroy(serviceName string) error
	DestroyUnits(unitNames ...string) error
	GetCharmURL(serviceName string) (*charm.URL, error)
	ModelUUID() string
}

func (c *removeServiceCommand) getAPI() (ServiceAPI, error) {
	root, err := c.NewAPIRoot()
	if err != nil {
		return nil, errors.Trace(err)
	}
	return apiservice.NewClient(root), nil
}

func (c *removeServiceCommand) Run(ctx *cmd.Context) error {
	client, err := c.getAPI()
	if err != nil {
		return err
	}
	defer client.Close()
	err = block.ProcessBlockedError(client.Destroy(c.ServiceName), block.BlockRemove)
	if err != nil {
		return err
	}
	return c.removeAllocation(ctx)
}

func (c *removeServiceCommand) removeAllocation(ctx *cmd.Context) error {
	client, err := c.getAPI()
	if err != nil {
		return err
	}
	charmURL, err := client.GetCharmURL(c.ServiceName)
	if err != nil {
		return errors.Trace(err)
	}
	if charmURL.Schema == "local" {
		return nil
	}

	root, err := c.NewAPIRoot()
	if err != nil {
		return errors.Trace(err)
	}
	charmsClient := charms.NewClient(root)
	metered, err := charmsClient.IsMetered(charmURL.String())
	if err != nil {
		return errors.Trace(err)
	}
	if !metered {
		return nil
	}

	modelUUID := client.ModelUUID()
	bakeryClient, err := c.BakeryClient()
	if err != nil {
		return errors.Trace(err)
	}
	budgetClient := getBudgetAPIClient(bakeryClient)

	resp, err := budgetClient.DeleteAllocation(modelUUID, c.ServiceName)
	if wireformat.IsNotAvail(err) {
		fmt.Fprintf(ctx.Stdout, "WARNING: Allocation not removed - %s.\n", err.Error())
	} else if err != nil {
		return err
	}
	if resp != "" {
		fmt.Fprintf(ctx.Stdout, "%s\n", resp)
	}
	return nil
}

var getBudgetAPIClient = getBudgetAPIClientImpl

func getBudgetAPIClientImpl(bakeryClient *httpbakery.Client) budgetAPIClient {
	return budget.NewClient(bakeryClient)
}

type budgetAPIClient interface {
	DeleteAllocation(string, string) (string, error)
}
