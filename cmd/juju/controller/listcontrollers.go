// Copyright 2015,2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package controller

import (
	"fmt"
	"strings"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	"launchpad.net/gnuflag"

	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/jujuclient"
)

// NewListControllersCommand returns a command to list registered controllers.
func NewListControllersCommand() cmd.Command {
	cmd := &listControllersCommand{
		store: jujuclient.NewFileClientStore(),
	}
	return modelcmd.WrapBase(cmd)
}

// Info implements Command.Info
func (c *listControllersCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "list-controllers",
		Purpose: "list all registered controllers",
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
	controllers, err := c.store.AllControllers()
	if err != nil {
		return errors.Annotate(err, "failed to list controllers")
	}
	details, errs := c.convertControllerDetails(controllers)
	if len(errs) > 0 {
		fmt.Fprintln(ctx.Stderr, strings.Join(errs, "\n"))
	}
	currentController, err := modelcmd.ReadCurrentController()
	if err != nil {
		return errors.Annotate(err, "getting current controller")
	}
	if _, ok := controllers[currentController]; !ok {
		// TODO(axw) move handling of current-controller to
		// the jujuclient code, and make sure the file is
		// kept in-sync with the controllers.yaml file.
		currentController = ""
	}
	controllerSet := ControllerSet{
		Controllers:       details,
		CurrentController: currentController,
	}
	return c.out.Write(ctx, controllerSet)
}

type listControllersCommand struct {
	modelcmd.JujuCommandBase

	out   cmd.Output
	store jujuclient.ClientStore
}

const listControllersDoc = `
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
