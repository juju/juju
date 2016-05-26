// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cloud

import (
	"github.com/juju/cmd"
	"github.com/juju/errors"

	"github.com/juju/juju/cloud"
)

var usageRemoveCloudSummary = `
Removes a user-defined cloud from Juju.`[1:]

var usageRemoveCloudDetails = `
Remove a named, user-defined cloud from Juju.

Examples:
    juju remove-cloud mycloud

See also:
    add-cloud
    list-clouds`

type removeCloudCommand struct {
	cmd.CommandBase

	// Cloud is the name fo the cloud to add.
	Cloud string
}

// NewRemoveCloudCommand returns a command to add cloud information.
func NewRemoveCloudCommand() cmd.Command {
	return &removeCloudCommand{}
}

func (c *removeCloudCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "remove-cloud",
		Args:    "<cloud name>",
		Purpose: usageRemoveCloudSummary,
		Doc:     usageRemoveCloudDetails,
	}
}

func (c *removeCloudCommand) Init(args []string) (err error) {
	if len(args) < 1 {
		return errors.New("Usage: juju remove-cloud <cloud name>")
	}
	c.Cloud = args[0]
	return cmd.CheckEmpty(args[1:])
}

func (c *removeCloudCommand) Run(ctxt *cmd.Context) error {
	personalClouds, err := cloud.PersonalCloudMetadata()
	if err != nil {
		return err
	}
	if _, ok := personalClouds[c.Cloud]; !ok {
		return errors.Errorf("personal cloud %q not found", c.Cloud)
	}
	delete(personalClouds, c.Cloud)
	return cloud.WritePersonalCloudMetadata(personalClouds)
}
