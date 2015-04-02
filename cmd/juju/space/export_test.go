// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package space

import "github.com/juju/cmd"

// EmptyCommand is a fake command that is used for testing ParseNameAndCIDRs.
type EmptyCommand struct {
	SpaceCommandBase
}

func (c *EmptyCommand) Init(args []string) error {
	return c.ParseNameAndCIDRs(args)
}

func (c *EmptyCommand) Info() *cmd.Info {
	return &cmd.Info{}
}

func (c *EmptyCommand) Run(ctx *cmd.Context) error {
	return nil
}

func NewEmptyCommand(api SpaceAPI) *EmptyCommand {
	cmd := &EmptyCommand{}
	cmd.api = api
	return cmd
}

func NewCreateCommand(api SpaceAPI) *CreateCommand {
	createCmd := &CreateCommand{}
	createCmd.api = api
	return createCmd
}

func NewRemoveCommand(api SpaceAPI) *RemoveCommand {
	removeCmd := &RemoveCommand{}
	removeCmd.api = api
	return removeCmd
}

func NewUpdateCommand(api SpaceAPI) *UpdateCommand {
	updateCmd := &UpdateCommand{}
	updateCmd.api = api
	return updateCmd
}

func NewRenameCommand(api SpaceAPI) *RenameCommand {
	updateCmd := &RenameCommand{}
	updateCmd.api = api
	return updateCmd
}
