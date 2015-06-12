// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package system

import (
	"fmt"
	"sort"

	"github.com/juju/cmd"
	"github.com/juju/errors"

	"github.com/juju/juju/environs/configstore"
)

// ListCommand returns the list of all systems the user is
// currently logged in to on the current machine.
type ListCommand struct {
	cmd.CommandBase
	cfgStore configstore.Storage
}

var listDoc = `
List all the Juju systems logged in to on the current machine.

A system refers to a Juju Environment System (JES) that runs and manages the
Juju API server and the underlying database used by Juju. A system may manage
multiple environments.

See Also:
    juju help juju-systems
    juju help system environments
    juju help system create-environment
    juju help system use-environment
`

// Info implements Command.Info
func (c *ListCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "list",
		Purpose: "list all systems logged in to on the current machine",
		Doc:     listDoc,
	}
}

func (c *ListCommand) getConfigstore() (configstore.Storage, error) {
	if c.cfgStore != nil {
		return c.cfgStore, nil
	}
	return configstore.Default()
}

// Run implements Command.Run
func (c *ListCommand) Run(ctx *cmd.Context) error {
	store, err := c.getConfigstore()

	if err != nil {
		return errors.Annotate(err, "failed to get config store")
	}

	list, err := store.ListSystems()
	if err != nil {
		return errors.Annotate(err, "failed to list systems in config store")
	}

	sort.Strings(list)
	for _, name := range list {
		fmt.Fprintf(ctx.Stdout, "%s\n", name)
	}

	return nil
}
