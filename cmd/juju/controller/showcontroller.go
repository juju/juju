// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package controller

import (
	"github.com/juju/cmd"
	"github.com/juju/errors"
	"launchpad.net/gnuflag"

	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/jujuclient"
)

// NewShowControllerCommand returns a command to show details of the desired controllers.
func NewShowControllerCommand() cmd.Command {
	cmd := &showControllerCommand{}
	cmd.newStoreFunc = func() (jujuclient.ControllerStore, error) {
		return jujuclient.DefaultControllerStore()
	}
	return modelcmd.WrapBase(cmd)
}

// Init implements Command.Init.
func (c *showControllerCommand) Init(args []string) (err error) {
	if len(args) < 1 {
		return errors.New("must specify controller name(s)")
	}
	c.controllerNames = args
	return nil
}

// Info implements Command.Info
func (c *showControllerCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "show-controller",
		Purpose: "show controller details for the given controller names",
		Doc:     showControllerDoc,
		Aliases: []string{"show-controllers"},
	}
}

// SetFlags implements Command.SetFlags.
func (c *showControllerCommand) SetFlags(f *gnuflag.FlagSet) {
	c.JujuCommandBase.SetFlags(f)
	c.out.AddFlags(f, "yaml", map[string]cmd.Formatter{
		"yaml": cmd.FormatYaml,
		"json": cmd.FormatJson,
	})
}

// Run implements Command.Run
func (c *showControllerCommand) Run(ctx *cmd.Context) error {
	store, err := c.newStoreFunc()
	if err != nil {
		return errors.Annotate(err, "failed to get jujuclient store")
	}

	controllers := make(map[string]jujuclient.ControllerDetails)
	for _, name := range c.controllerNames {
		one, err := store.ControllerByName(name)
		if err != nil {
			return errors.Annotatef(err, "failed to get controller %q from jujuclient store", name)
		}
		controllers[name] = *one
	}
	return c.out.Write(ctx, controllers)
}

type showControllerCommand struct {
	modelcmd.JujuCommandBase

	out          cmd.Output
	newStoreFunc func() (jujuclient.ControllerStore, error)

	controllerNames []string
}

const showControllerDoc = `
Show extended information about controller(s).
Controllers to display are specified by controller names.

arguments:
[space separated controller names]
`
