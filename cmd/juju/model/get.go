// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package model

import (
	"fmt"
	"strings"

	"github.com/juju/cmd"
	"launchpad.net/gnuflag"

	"github.com/juju/juju/cmd/modelcmd"
)

func NewGetCommand() cmd.Command {
	return modelcmd.Wrap(&getCommand{})
}

// getCommand is able to output either the entire environment or
// the requested value in a format of the user's choosing.
type getCommand struct {
	modelcmd.ModelCommandBase
	api GetEnvironmentAPI
	key string
	out cmd.Output
}

const getModelHelpDoc = `
If no extra args passed on the command line, all configuration keys and values
for the environment are output using the selected formatter.

A single model value can be output by adding the model key name to
the end of the command line.

Example:
  
  juju get-model-config default-series  (returns the default series for the model)
`

func (c *getCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "get-model-config",
		Args:    "[<model key>]",
		Purpose: "view model values",
		Doc:     strings.TrimSpace(getModelHelpDoc),
	}
}

func (c *getCommand) SetFlags(f *gnuflag.FlagSet) {
	c.out.AddFlags(f, "smart", cmd.DefaultFormatters)
}

func (c *getCommand) Init(args []string) (err error) {
	c.key, err = cmd.ZeroOrOneArgs(args)
	return
}

type GetEnvironmentAPI interface {
	Close() error
	ModelGet() (map[string]interface{}, error)
}

func (c *getCommand) getAPI() (GetEnvironmentAPI, error) {
	if c.api != nil {
		return c.api, nil
	}
	return c.NewAPIClient()
}

func (c *getCommand) Run(ctx *cmd.Context) error {
	client, err := c.getAPI()
	if err != nil {
		return err
	}
	defer client.Close()

	attrs, err := client.ModelGet()
	if err != nil {
		return err
	}

	if c.key != "" {
		if value, found := attrs[c.key]; found {
			return c.out.Write(ctx, value)
		}
		return fmt.Errorf("key %q not found in %q model.", c.key, attrs["name"])
	}
	// If key is empty, write out the whole lot.
	return c.out.Write(ctx, attrs)
}
