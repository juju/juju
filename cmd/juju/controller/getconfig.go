// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package controller

import (
	"strings"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"

	apicontroller "github.com/juju/juju/api/controller"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/cmd/output"
	"github.com/juju/juju/controller"
)

func NewGetConfigCommand() cmd.Command {
	return modelcmd.WrapController(&getConfigCommand{})
}

// getConfigCommand is able to output either the entire environment or
// the requested value in a format of the user's choosing.
type getConfigCommand struct {
	modelcmd.ControllerCommandBase
	api controllerAPI
	key string
	out cmd.Output
}

const getControllerHelpDoc = `
By default, all configuration (keys and values) for the controller are
displayed if a key is not specified.

Examples:

    juju get-controller-config
    juju get-controller-config api-port
    juju get-controller-config -c mycontroller

See also:
    controllers
`

func (c *getConfigCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "get-controller-config",
		Args:    "[<attribute key>]",
		Purpose: "Displays configuration settings for a controller.",
		Doc:     strings.TrimSpace(getControllerHelpDoc),
	}
}

func (c *getConfigCommand) SetFlags(f *gnuflag.FlagSet) {
	c.ControllerCommandBase.SetFlags(f)
	c.out.AddFlags(f, "yaml", output.DefaultFormatters)
}

func (c *getConfigCommand) Init(args []string) (err error) {
	c.key, err = cmd.ZeroOrOneArgs(args)
	return
}

type controllerAPI interface {
	Close() error
	ControllerConfig() (controller.Config, error)
}

func (c *getConfigCommand) getAPI() (controllerAPI, error) {
	if c.api != nil {
		return c.api, nil
	}
	root, err := c.NewAPIRoot()
	if err != nil {
		return nil, errors.Trace(err)
	}
	return apicontroller.NewClient(root), nil
}

func (c *getConfigCommand) Run(ctx *cmd.Context) error {
	client, err := c.getAPI()
	if err != nil {
		return err
	}
	defer client.Close()

	attrs, err := client.ControllerConfig()
	if err != nil {
		return err
	}

	if c.key != "" {
		if value, found := attrs[c.key]; found {
			return c.out.Write(ctx, value)
		}
		return errors.Errorf("key %q not found in %q controller.", c.key, c.ControllerName())
	}
	// If key is empty, write out the whole lot.
	return c.out.Write(ctx, attrs)
}
