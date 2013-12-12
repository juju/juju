// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"fmt"

	"launchpad.net/gnuflag"

	"launchpad.net/juju-core/charm"
	"launchpad.net/juju-core/cmd"
	"launchpad.net/juju-core/store"
)

type DeleteCharmCommand struct {
	ConfigCommand
	Url string
}

func (c *DeleteCharmCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "delete-charm",
		Purpose: "delete a published charm from the charm store",
	}
}

func (c *DeleteCharmCommand) SetFlags(f *gnuflag.FlagSet) {
	c.ConfigCommand.SetFlags(f)
	f.StringVar(&c.Url, "url", "", "charm URL")
}

func (c *DeleteCharmCommand) Init(args []string) error {
	// Check flags
	err := c.ConfigCommand.Init(args)
	if err != nil {
		return err
	}
	if c.Url == "" {
		return fmt.Errorf("--url is required")
	}
	return nil
}

func (c *DeleteCharmCommand) Run(ctx *cmd.Context) error {
	// Read config
	err := c.ConfigCommand.ReadConfig(ctx)
	if err != nil {
		return err
	}

	// Parse the charm URL
	charmUrl, err := charm.ParseURL(c.Url)
	if err != nil {
		return err
	}

	// Open the charm store storage
	s, err := store.Open(c.Config.MongoURL)
	if err != nil {
		return err
	}
	defer s.Close()

	// Delete the charm by URL
	_, err = s.DeleteCharm(charmUrl)
	if err != nil {
		return err
	}
	fmt.Fprintln(ctx.Stdout, "Charm", charmUrl, "deleted.")
	return nil
}

func (c *DeleteCharmCommand) AllowInterspersedFlags() bool {
	return true
}
