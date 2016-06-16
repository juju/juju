// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package controller

import (
	"fmt"
	"strings"

	"github.com/juju/cmd"
	"launchpad.net/gnuflag"

	"github.com/juju/errors"
	apicontroller "github.com/juju/juju/api/controller"
	"github.com/juju/juju/cmd/modelcmd"
)

func NewGetSharedConfigCommand() cmd.Command {
	return modelcmd.WrapController(&getSharedConfigCommand{})
}

// getSharedConfigCommand is able to output either the entire default
// model config settings or the requested value in a format of the
// user's choosing.
type getSharedConfigCommand struct {
	modelcmd.ControllerCommandBase
	api controllerSharedConfigAPI
	key string
	out cmd.Output
}

const getSharedConfigHelpDoc = `
By default, all configuration (keys and values) used as the
defaults for all models are displayed if a key is not specified.

Examples:

    juju get-model-defaults
    juju get-model-defaults apt-mirror
    juju get-model-defaults -c mycontroller

See also:
    set-model-defaults
    unset-model-defaults
    get-model-config
`

func (c *getSharedConfigCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "get-model-defaults",
		Args:    "[<attribute key>]",
		Purpose: "Displays default configuration settings for a controller's hosted models.",
		Doc:     strings.TrimSpace(getSharedConfigHelpDoc),
	}
}

func (c *getSharedConfigCommand) SetFlags(f *gnuflag.FlagSet) {
	c.out.AddFlags(f, "smart", cmd.DefaultFormatters)
}

func (c *getSharedConfigCommand) Init(args []string) (err error) {
	c.key, err = cmd.ZeroOrOneArgs(args)
	return
}

type controllerSharedConfigAPI interface {
	Close() error
	DefaultModelConfig() (map[string]interface{}, error)
}

func (c *getSharedConfigCommand) getAPI() (controllerSharedConfigAPI, error) {
	if c.api != nil {
		return c.api, nil
	}
	root, err := c.NewAPIRoot()
	if err != nil {
		return nil, errors.Trace(err)
	}
	return apicontroller.NewClient(root), nil
}

func (c *getSharedConfigCommand) Run(ctx *cmd.Context) error {
	client, err := c.getAPI()
	if err != nil {
		return err
	}
	defer client.Close()

	attrs, err := client.DefaultModelConfig()
	if err != nil {
		return err
	}

	if c.key != "" {
		if value, found := attrs[c.key]; found {
			return c.out.Write(ctx, value)
		}
		return fmt.Errorf("key %q not found in %q default model settings.", c.key, c.ControllerName())
	}
	// If key is empty, write out the whole lot.
	return c.out.Write(ctx, attrs)
}
