// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package application

import (
	"github.com/juju/cmd"
	"github.com/juju/errors"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/api/application"
	"github.com/juju/juju/cmd/juju/block"
	"github.com/juju/juju/cmd/modelcmd"
)

// NewRemoveApplicationCommand returns a command which removes an application.
func NewRemoveApplicationCommand() cmd.Command {
	return modelcmd.Wrap(&removeApplicationCommand{})
}

// removeServiceCommand causes an existing application to be destroyed.
type removeApplicationCommand struct {
	modelcmd.ModelCommandBase
	ApplicationName string
}

var helpSummaryRmApp = `
Remove an application from the model.`[1:]

var helpDetailsRmApp = `
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

func (c *removeApplicationCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "remove-application",
		Args:    "<application>",
		Purpose: helpSummaryRmApp,
		Doc:     helpDetailsRmApp,
	}
}

func (c *removeApplicationCommand) Init(args []string) error {
	if len(args) == 0 {
		return errors.Errorf("no application specified")
	}
	if !names.IsValidApplication(args[0]) {
		return errors.Errorf("invalid application name %q", args[0])
	}
	c.ApplicationName, args = args[0], args[1:]
	return cmd.CheckEmpty(args)
}

type removeApplicationAPI interface {
	Close() error
	Destroy(serviceName string) error
	DestroyUnits(unitNames ...string) error
	ModelUUID() string
}

func (c *removeApplicationCommand) getAPI() (removeApplicationAPI, error) {
	root, err := c.NewAPIRoot()
	if err != nil {
		return nil, errors.Trace(err)
	}
	return application.NewClient(root), nil
}

func (c *removeApplicationCommand) Run(ctx *cmd.Context) error {
	client, err := c.getAPI()
	if err != nil {
		return err
	}
	defer client.Close()
	err = block.ProcessBlockedError(client.Destroy(c.ApplicationName), block.BlockRemove)
	return err
}
