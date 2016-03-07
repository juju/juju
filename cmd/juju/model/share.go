// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package model

import (
	"strings"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/names"
	"launchpad.net/gnuflag"

	"github.com/juju/juju/cmd/juju/block"
	"github.com/juju/juju/cmd/modelcmd"
)

const shareEnvHelpDoc = `
Share the current model with another user.

Examples:
 juju share-model joe
     Give local user "joe" default (write) access to the current model

 juju share-model --acl read joe
     Give local user "joe" read access to the current model

 juju share-model user1 user2 user3@ubuntuone
     Give two local users and one remote user admin access to the current model

 juju share-model sam --model myenv --acl write
     Give local user "sam" write access to the model named "myenv"
 `

func NewShareCommand() cmd.Command {
	return modelcmd.Wrap(&shareCommand{})
}

// shareCommand represents the command to share an environment with a user(s).
type shareCommand struct {
	modelcmd.ModelCommandBase
	envName string
	api     ShareEnvironmentAPI

	// Users to share the model with.
	Users []names.UserTag

	// Permission users have when accessing the model.
	ModelAccess string
}

func (c *shareCommand) SetFlags(f *gnuflag.FlagSet) {
	f.StringVar(&c.ModelAccess, "acl", "write", "model access permissions")
}

// Info implements Command.Info.
func (c *shareCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "share-model",
		Args:    "<user> ...",
		Purpose: "share the current model with another user",
		Doc:     strings.TrimSpace(shareEnvHelpDoc),
	}
}

func (c *shareCommand) Init(args []string) (err error) {
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

func (c *shareCommand) getAPI() (ShareEnvironmentAPI, error) {
	if c.api != nil {
		return c.api, nil
	}
	return c.NewAPIClient()
}

// ShareEnvironmentAPI defines the API functions used by the environment share command.
type ShareEnvironmentAPI interface {
	Close() error
	ShareModel(access string, users ...names.UserTag) error
}

func (c *shareCommand) Run(ctx *cmd.Context) error {
	client, err := c.getAPI()
	if err != nil {
		return err
	}
	defer client.Close()

	return block.ProcessBlockedError(client.ShareModel(c.ModelAccess, c.Users...), block.BlockChange)
}
