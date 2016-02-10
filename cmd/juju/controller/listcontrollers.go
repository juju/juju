// Copyright 2015,2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package controller

import (
	"github.com/juju/cmd"
	"github.com/juju/errors"
	"launchpad.net/gnuflag"

	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/jujuclient"
)

// NewListControllersCommand returns a command to list the controllers the user knows about.
func NewListControllersCommand() cmd.Command {
	cmd := &listControllersCommand{}
	cmd.newStoreFunc = func() (jujuclient.ControllerStore, error) {
		return jujuclient.DefaultControllerStore()
	}
	return modelcmd.WrapBase(cmd)
}

// Info implements Command.Info
func (c *listControllersCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "list-controllers",
		Purpose: "list all controllers logged in to on the current machine",
		Doc:     listControllersDoc,
	}
}

// SetFlags implements Command.SetFlags.
func (c *listControllersCommand) SetFlags(f *gnuflag.FlagSet) {
	c.JujuCommandBase.SetFlags(f)
	c.out.AddFlags(f, "tabular", map[string]cmd.Formatter{
		"yaml":    cmd.FormatYaml,
		"json":    cmd.FormatJson,
		"tabular": formatControllersListTabular,
	})
}

// Run implements Command.Run
func (c *listControllersCommand) Run(ctx *cmd.Context) error {
	store, err := c.newStoreFunc()
	if err != nil {
		return errors.Annotate(err, "failed to get jujuclient store")
	}

	controllers, err := store.AllControllers()
	if err != nil {
		return errors.Annotate(err, "failed to list controllers in jujuclient store")
	}
	if len(controllers) == 0 {
		return nil
	}

	// TODO (anastasiamac 2016-02-10) get the most recently used model per controller,
	// preferably bulk call that takes all controller names and returns corresponding models...
	details := convertControllerDetails(controllers)
	if err != nil {
		return err
	}
	return c.out.Write(ctx, details)
}

type listControllersCommand struct {
	modelcmd.JujuCommandBase

	out          cmd.Output
	newStoreFunc func() (jujuclient.ControllerStore, error)
}

var listControllersDoc = `
A controller refers to a Juju Controller that runs and manages the Juju API
server and the underlying database used by Juju. A controller may host
multiple models.

options:
-o, --output (= "")
   specify an output file
--format (= tabular)
   specify output format (json|tabular|yaml)

See Also:
    juju help controllers
    juju help list-models
    juju help create-model
    juju help use-model
`
