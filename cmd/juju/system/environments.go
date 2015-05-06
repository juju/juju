// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package system

import (
	"fmt"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	"launchpad.net/gnuflag"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/cmd/syscmd"
	"github.com/juju/juju/environs/configstore"
)

// EnvironmentsCommand returns the list of all the environments the
// current user can access on the current system.
type EnvironmentsCommand struct {
	syscmd.SysCommandBase
	user      string
	envmgrAPI EnvMgrAPI
	userCreds *configstore.APICredentials
}

var envsDoc = `List all the environments the user can access on the current system`

// EnvMgrAPI defines the methods on the client API that the
// environments command calls.
type EnvMgrAPI interface {
	Close() error
	ListEnvironments(user string) ([]params.Environment, error)
}

// Info implements Command.Info
func (c *EnvironmentsCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "environments",
		Purpose: "list all environments the user can access on the current system",
		Doc:     envsDoc,
	}
}

func (c *EnvironmentsCommand) getEnvMgrAPIClient() (EnvMgrAPI, error) {
	if c.envmgrAPI != nil {
		return c.envmgrAPI, nil
	}
	return c.NewEnvMgrAPIClient()
}

func (c *EnvironmentsCommand) getConnectionCredentials() (configstore.APICredentials, error) {
	if c.userCreds != nil {
		return *c.userCreds, nil
	}
	return c.ConnectionCredentials()
}

// SetFlags implements Command.SetFlags.
func (c *EnvironmentsCommand) SetFlags(f *gnuflag.FlagSet) {
	f.StringVar(&c.user, "user", "", "the user to list environments for.  Only administrative users can list environments for other users.")
}

// Run implements Command.Run
func (c *EnvironmentsCommand) Run(ctx *cmd.Context) error {
	client, err := c.getEnvMgrAPIClient()
	if err != nil {
		return errors.Trace(err)
	}
	defer client.Close()

	if c.user == "" {
		creds, err := c.getConnectionCredentials()
		if err != nil {
			return errors.Trace(err)
		}
		c.user = creds.User
	}

	envs, err := client.ListEnvironments(c.user)
	if err != nil {
		return errors.Annotate(err, "cannot list environments")
	}

	for _, env := range envs {
		fmt.Fprintf(ctx.Stdout, "%s\n", env.Name)
	}

	return nil
}
