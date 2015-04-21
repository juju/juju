// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package system

import (
	"fmt"

	"github.com/juju/cmd"
	"github.com/juju/errors"

	"github.com/juju/juju/environs/configstore"
)

type ListCommand struct {
	cmd.CommandBase
	cfgStore configstore.Storage
}

var switchDoc = `List all the systems logged in to on the current machine`

func (c *ListCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "list",
		Purpose: "list all systems logged in to on the current machine",
		Doc:     switchDoc,
	}
}

func (c *ListCommand) getConfigstore() (configstore.Storage, error) {
	if c.cfgStore != nil {
		return c.cfgStore, nil
	}
	return configstore.Default()
}

func (c *ListCommand) Run(ctx *cmd.Context) error {
	store, err := c.getConfigstore()

	if err != nil {
		return errors.Annotate(err, "failed to get config store")
	}

	list, err := store.ListServers()
	if err != nil {
		return errors.Annotate(err, "failed to list systems in config store")
	}

	for _, name := range list {
		fmt.Fprintf(ctx.Stdout, "%s\n", name)
	}

	return nil
}
