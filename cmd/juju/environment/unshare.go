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

const unshareEnvHelpDoc = `
Deny a user access to an environment that was previously shared with them.

Examples:
 juju environment unshare joe
     Deny local user "joe" access to the current environment

 juju environment unshare user1 user2 user3@ubuntuone
     Deny two local users and one remote user access to the current environment

 juju environment unshare sam -e/--environment myenv
     Deny local user "sam" access to the environment named "myenv"
 `

// UnshareCommand unshares an environment with the given user(s).
type UnshareCommand struct {
	envcmd.EnvCommandBase
	cmd.CommandBase
	envName string
	api     UnshareEnvironmentAPI

	// Users to unshare the environment with.
	Users []names.UserTag
}

func (c *UnshareCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "unshare",
		Args:    "<user> ...",
		Purpose: "unshare the current environment with a user",
		Doc:     strings.TrimSpace(unshareEnvHelpDoc),
	}
}

func (c *UnshareCommand) Init(args []string) (err error) {
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

func (c *UnshareCommand) getAPI() (UnshareEnvironmentAPI, error) {
	if c.api != nil {
		return c.api, nil
	}
	return c.NewAPIClient()
}

// UnshareEnvironmentAPI defines the API functions used by the environment
// unshare command.
type UnshareEnvironmentAPI interface {
	Close() error
	UnshareEnvironment(...names.UserTag) error
}

func (c *UnshareCommand) Run(ctx *cmd.Context) error {
	client, err := c.getAPI()
	if err != nil {
		return err
	}
	defer client.Close()

	return block.ProcessBlockedError(client.UnshareEnvironment(c.Users...), block.BlockChange)
}
