// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package environment

import (
	"strings"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/names"

	"github.com/juju/juju/cmd/envcmd"
	"github.com/juju/juju/cmd/juju/block"
)

const shareEnvHelpDoc = `
Share the current environment with another user.

Examples:
 juju environment share joe
     Give local user "joe" access to the current environment

 juju environment share user1 user2 user3@ubuntuone
     Give two local users and one remote user access to the current environment

 juju environment share sam --environment myenv
     Give local user "sam" access to the environment named "myenv"
 `

// ShareCommand represents the command to share an environment with a user(s).
type ShareCommand struct {
	envcmd.EnvCommandBase
	envName string
	api     ShareEnvironmentAPI

	// Users to share the environment with.
	Users []names.UserTag
}

// Info implements Command.Info.
func (c *ShareCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "share",
		Args:    "<user> ...",
		Purpose: "share the current environment with another user",
		Doc:     strings.TrimSpace(shareEnvHelpDoc),
	}
}

func (c *ShareCommand) Init(args []string) (err error) {
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

func (c *ShareCommand) getAPI() (ShareEnvironmentAPI, error) {
	if c.api != nil {
		return c.api, nil
	}
	return c.NewAPIClient()
}

// ShareEnvironmentAPI defines the API functions used by the environment share command.
type ShareEnvironmentAPI interface {
	Close() error
	ShareEnvironment(...names.UserTag) error
}

func (c *ShareCommand) Run(ctx *cmd.Context) error {
	client, err := c.getAPI()
	if err != nil {
		return err
	}
	defer client.Close()

	return block.ProcessBlockedError(client.ShareEnvironment(c.Users...), block.BlockChange)
}
