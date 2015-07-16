// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package commands

import (
	"fmt"

	"github.com/juju/cmd"
	"github.com/juju/names"

	"github.com/juju/juju/cmd/envcmd"
	"github.com/juju/juju/cmd/juju/block"
)

// RemoveUnitCommand is responsible for destroying service units.
type RemoveUnitCommand struct {
	envcmd.EnvCommandBase
	UnitNames []string
}

const removeUnitDoc = `
Remove service units from the environment.

If this is the only unit running, the machine on which
the unit is hosted will also be destroyed, if possible.
The machine will be destroyed if:
- it is not a state server
- it is not hosting any Juju managed containers
`

func (c *RemoveUnitCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "remove-unit",
		Args:    "<unit> [...]",
		Purpose: "remove service units from the environment",
		Doc:     removeUnitDoc,
		Aliases: []string{"destroy-unit"},
	}
}

func (c *RemoveUnitCommand) Init(args []string) error {
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

// Run connects to the environment specified on the command line and destroys
// units therein.
func (c *RemoveUnitCommand) Run(_ *cmd.Context) error {
	client, err := c.NewAPIClient()
	if err != nil {
		return err
	}
	defer client.Close()
	return block.ProcessBlockedError(client.DestroyServiceUnits(c.UnitNames...), block.BlockRemove)
}
