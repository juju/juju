// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package space

import (
	"github.com/juju/cmd"

	"github.com/juju/juju/cmd/modelcmd"
)

func NewCreateCommand(api SpaceAPI) cmd.Command {
	createCmd := &createCommand{
		SpaceCommandBase: SpaceCommandBase{api: api},
	}
	return modelcmd.Wrap(createCmd)
}

type RemoveCommand struct {
	*removeCommand
}

func (c *RemoveCommand) Name() string {
	return c.name
}

func NewRemoveCommand(api SpaceAPI) (cmd.Command, *RemoveCommand) {
	removeCmd := &removeCommand{
		SpaceCommandBase: SpaceCommandBase{api: api},
	}
	return modelcmd.Wrap(removeCmd), &RemoveCommand{removeCmd}
}

func NewUpdateCommand(api SpaceAPI) cmd.Command {
	updateCmd := &updateCommand{
		SpaceCommandBase: SpaceCommandBase{api: api},
	}
	return modelcmd.Wrap(updateCmd)
}

type RenameCommand struct {
	*renameCommand
}

func NewRenameCommand(api SpaceAPI) (cmd.Command, *RenameCommand) {
	renameCmd := &renameCommand{
		SpaceCommandBase: SpaceCommandBase{api: api},
	}
	return modelcmd.Wrap(renameCmd), &RenameCommand{renameCmd}
}

type ListCommand struct {
	*listCommand
}

func (c *ListCommand) ListFormat() string {
	return c.out.Name()
}

func NewListCommand(api SpaceAPI) (cmd.Command, *ListCommand) {
	listCmd := &listCommand{
		SpaceCommandBase: SpaceCommandBase{api: api},
	}
	return modelcmd.Wrap(listCmd), &ListCommand{listCmd}
}
