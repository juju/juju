// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package application

import (
	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/romulus/api/budget"
	wireformat "github.com/juju/romulus/wireformat/budget"
	"gopkg.in/juju/charm.v6-unstable"
	"gopkg.in/juju/names.v2"
	"gopkg.in/macaroon-bakery.v1/httpbakery"

	"github.com/juju/juju/api/application"
	"github.com/juju/juju/api/charms"
	"github.com/juju/juju/cmd/juju/block"
	"github.com/juju/juju/cmd/modelcmd"
)

// NewRemoveServiceCommand returns a command which removes an application.
func NewRemoveServiceCommand() cmd.Command {
	return modelcmd.Wrap(&removeServiceCommand{})
}

// removeServiceCommand causes an existing application to be destroyed.
type removeServiceCommand struct {
	modelcmd.ModelCommandBase
	ApplicationName string
}

var helpSummaryRmSvc = `
Remove an application from the model.`[1:]

var helpDetailsRmSvc = `
Removing an application will terminate any relations that application has, remove
all units of the application, and in the case that this leaves machines with
no running applications, Juju will also remove the machine. For this reason,
you should retrieve any logs or data required from applications and units 
before removing them. Removing units which are co-located with units of
other charms or a Juju controller will not result in the removal of the
machine.

Examples:
    juju remove-application hadoop
    juju remove-application -m test-model mariadb`[1:]

func (c *removeServiceCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "remove-application",
		Args:    "<application>",
		Purpose: helpSummaryRmSvc,
		Doc:     helpDetailsRmSvc,
	}
}

func (c *removeServiceCommand) Init(args []string) error {
	if len(args) == 0 {
		return errors.Errorf("no application specified")
	}
	if !names.IsValidApplication(args[0]) {
		return errors.Errorf("invalid application name %q", args[0])
	}
	c.ApplicationName, args = args[0], args[1:]
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
	return application.NewClient(root), nil
}

func (c *removeServiceCommand) Run(ctx *cmd.Context) error {
	client, err := c.getAPI()
	if err != nil {
		return err
	}
	defer client.Close()
	err = block.ProcessBlockedError(client.Destroy(c.ApplicationName), block.BlockRemove)
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
	charmURL, err := client.GetCharmURL(c.ApplicationName)
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

	resp, err := budgetClient.DeleteAllocation(modelUUID, c.ApplicationName)
	if wireformat.IsNotAvail(err) {
		logger.Warningf("allocation not removed: %v", err)
	} else if err != nil {
		return err
	}
	if resp != "" {
		logger.Infof(resp)
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
