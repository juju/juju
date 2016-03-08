// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package subnet

import (
	"github.com/juju/cmd"

	"github.com/juju/juju/cmd/modelcmd"
)

type CreateCommand struct {
	*createCommand
}

func NewCreateCommand(api SubnetAPI) (cmd.Command, *CreateCommand) {
	cmd := &createCommand{
		SubnetCommandBase: SubnetCommandBase{api: api},
	}
	return modelcmd.Wrap(cmd), &CreateCommand{cmd}
}

type AddCommand struct {
	*addCommand
}

func NewAddCommand(api SubnetAPI) (cmd.Command, *AddCommand) {
	cmd := &addCommand{
		SubnetCommandBase: SubnetCommandBase{api: api},
	}
	return modelcmd.Wrap(cmd), &AddCommand{cmd}
}

type RemoveCommand struct {
	*removeCommand
}

func NewRemoveCommand(api SubnetAPI) (cmd.Command, *RemoveCommand) {
	removeCmd := &removeCommand{
		SubnetCommandBase: SubnetCommandBase{api: api},
	}
	return modelcmd.Wrap(removeCmd), &RemoveCommand{removeCmd}
}

type ListCommand struct {
	*listCommand
}

func NewListCommand(api SubnetAPI) (cmd.Command, *ListCommand) {
	cmd := &listCommand{
		SubnetCommandBase: SubnetCommandBase{api: api},
	}
	return modelcmd.Wrap(cmd), &ListCommand{cmd}
}
