// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package space

import (
	"github.com/juju/cmd"

	"github.com/juju/juju/cmd/modelcmd"
)

func NewAddCommandForTest(api SpaceAPI) cmd.Command {
	addCmd := &addCommand{
		SpaceCommandBase: SpaceCommandBase{api: api},
	}
	return modelcmd.Wrap(addCmd)
}

type RemoveCommand struct {
	*removeCommand
}

func (c *RemoveCommand) Name() string {
	return c.name
}

func NewRemoveCommandForTest(api SpaceAPI) (cmd.Command, *RemoveCommand) {
	removeCmd := &removeCommand{
		SpaceCommandBase: SpaceCommandBase{api: api},
	}
	return modelcmd.Wrap(removeCmd), &RemoveCommand{removeCmd}
}

func NewUpdateCommandForTest(api SpaceAPI) cmd.Command {
	updateCmd := &updateCommand{
		SpaceCommandBase: SpaceCommandBase{api: api},
	}
	return modelcmd.Wrap(updateCmd)
}

type RenameCommand struct {
	*renameCommand
}

func NewRenameCommandForTest(api SpaceAPI) (cmd.Command, *RenameCommand) {
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

func NewListCommandForTest(api SpaceAPI) (cmd.Command, *ListCommand) {
	listCmd := &listCommand{
		SpaceCommandBase: SpaceCommandBase{api: api},
	}
	return modelcmd.Wrap(listCmd), &ListCommand{listCmd}
}
