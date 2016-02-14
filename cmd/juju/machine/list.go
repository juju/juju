// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package machine

import (
	"github.com/juju/cmd"
	"github.com/juju/errors"

	"github.com/juju/juju/cmd/modelcmd"
)

const listMachinesCommandDoc = `
List all the machines in a juju model.
Default display is in tabular format with the following sections:
ID, STATE, DNS, INS-ID, SERIES, AZ

Note: AZ above is the cloud region's availability zone.
`

// NewListMachineCommand returns a command that lists the machines in a model.
func NewListMachinesCommand() cmd.Command {
	return modelcmd.Wrap(newListMachinesCommand(nil))
}

func newListMachinesCommand(api statusAPI) *listMachinesCommand {
	listCmd := &listMachinesCommand{}
	listCmd.defaultFormat = "tabular"
	listCmd.api = api
	return listCmd
}

// listMachineCommand holds infomation about machines in a model.
type listMachinesCommand struct {
	baselistMachinesCommand
}

// Info implements Command.Info.
func (c *listMachinesCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "list-machines",
		Purpose: "list machines in a model",
		Doc:     listMachinesCommandDoc,
		Aliases: []string{"machines", "machine", "list-machine"},
	}
}

// Init ensures the list-machines Command does not take arguments.
func (c *listMachinesCommand) Init(args []string) (err error) {
	if args != nil {
		return errors.Errorf("The list-machines command does not take any arguments")
	}
	return nil
}
