// Copyright 2012, 2013 Canonical Ltd.
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

// NewRemoveUnitCommand returns a command which removes a service's units.
func NewRemoveUnitCommand() cmd.Command {
	return modelcmd.Wrap(&removeUnitCommand{})
}

// removeUnitCommand is responsible for destroying service units.
type removeUnitCommand struct {
	modelcmd.ModelCommandBase
	UnitNames []string
}

const removeUnitDoc = `
Remove service units from the model.

If this is the only unit running, the machine on which
the unit is hosted will also be destroyed, if possible.
The machine will be destroyed if:
- it is not a controller
- it is not hosting any Juju managed containers
`

func (c *removeUnitCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "remove-unit",
		Args:    "<unit> [...]",
		Purpose: "remove service units from the model",
		Doc:     removeUnitDoc,
		Aliases: []string{"destroy-unit"},
	}
}

func (c *removeUnitCommand) Init(args []string) error {
	c.UnitNames = args
	if len(c.UnitNames) == 0 {
		return fmt.Errorf("no units specified")
	}
	for _, name := range c.UnitNames {
		if !names.IsValidUnit(name) {
			return fmt.Errorf("invalid unit name %q", name)
		}
	}
	return nil
}

func (c *removeUnitCommand) getAPI() (ServiceAPI, error) {
	root, err := c.NewAPIRoot()
	if err != nil {
		return nil, errors.Trace(err)
	}
	return apiservice.NewClient(root), nil
}

// Run connects to the environment specified on the command line and destroys
// units therein.
func (c *removeUnitCommand) Run(_ *cmd.Context) error {
	client, err := c.getAPI()
	if err != nil {
		return err
	}
	defer client.Close()
	return block.ProcessBlockedError(client.DestroyUnits(c.UnitNames...), block.BlockRemove)
}
