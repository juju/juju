// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package model

import (
	"strings"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/names"

	"github.com/juju/juju/cmd/juju/block"
	"github.com/juju/juju/cmd/modelcmd"
)

const unshareEnvHelpDoc = `
Deny a user access to an model that was previously shared with them.

Examples:
 juju unshare-model joe
     Deny local user "joe" access to the current model

 juju unshare-model user1 user2 user3@ubuntuone
     Deny two local users and one remote user access to the current model

 juju unshare-model sam -m/--model myenv
     Deny local user "sam" access to the model named "mymodel"
 `

func NewUnshareCommand() cmd.Command {
	return modelcmd.Wrap(&unshareCommand{})
}

// unshareCommand unshares an environment with the given user(s).
type unshareCommand struct {
	modelcmd.ModelCommandBase
	cmd.CommandBase
	envName string
	api     UnshareEnvironmentAPI

	// Users to unshare the environment with.
	Users []names.UserTag
}

func (c *unshareCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "unshare-model",
		Args:    "<user> ...",
		Purpose: "unshare the current model with a user",
		Doc:     strings.TrimSpace(unshareEnvHelpDoc),
	}
}

func (c *unshareCommand) Init(args []string) (err error) {
	if len(args) == 0 {
		return errors.New("no users specified")
	}

	for _, arg := range args {
		if !names.IsValidUser(arg) {
			return errors.Errorf("invalid username: %q", arg)
		}
		c.Users = append(c.Users, names.NewUserTag(arg))
	}

	return nil
}

func (c *unshareCommand) getAPI() (UnshareEnvironmentAPI, error) {
	if c.api != nil {
		return c.api, nil
	}
	return c.NewAPIClient()
}

// UnshareEnvironmentAPI defines the API functions used by the environment
// unshare command.
type UnshareEnvironmentAPI interface {
	Close() error
	UnshareModel(...names.UserTag) error
}

func (c *unshareCommand) Run(ctx *cmd.Context) error {
	client, err := c.getAPI()
	if err != nil {
		return err
	}
	defer client.Close()

	return block.ProcessBlockedError(client.UnshareModel(c.Users...), block.BlockChange)
}
