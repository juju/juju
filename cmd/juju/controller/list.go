// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package controller

import (
	"fmt"
	"sort"

	"github.com/juju/cmd"
	"github.com/juju/errors"

	"github.com/juju/juju/cmd/envcmd"
	"github.com/juju/juju/environs/configstore"
)

// NewListCommand returns a command to list the controllers the user knows about.
func NewListCommand() cmd.Command {
	return envcmd.WrapBase(&listCommand{})
}

// listCommand returns the list of all controllers the user is
// currently logged in to on the current machine.
type listCommand struct {
	envcmd.JujuCommandBase
	cfgStore configstore.Storage
}

var listDoc = `
List all the Juju controllers logged in to on the current machine.

A controller refers to a Juju Controller that runs and manages the Juju API
server and the underlying database used by Juju. A controller may host
multiple environments.

See Also:
    juju help controllers
    juju help list-environments
    juju help create-environment
    juju help use-environment
`

// Info implements Command.Info
func (c *listCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "list-controllers",
		Purpose: "list all controllers logged in to on the current machine",
		Doc:     listDoc,
	}
}

func (c *listCommand) getConfigstore() (configstore.Storage, error) {
	if c.cfgStore != nil {
		return c.cfgStore, nil
	}
	return configstore.Default()
}

// Run implements Command.Run
func (c *listCommand) Run(ctx *cmd.Context) error {
	store, err := c.getConfigstore()

	if err != nil {
		return errors.Annotate(err, "failed to get config store")
	}

	list, err := store.ListSystems()
	if err != nil {
		return errors.Annotate(err, "failed to list controllers in config store")
	}

	sort.Strings(list)
	for _, name := range list {
		fmt.Fprintf(ctx.Stdout, "%s\n", name)
	}

	return nil
}
