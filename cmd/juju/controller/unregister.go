// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package controller

import (
	"fmt"

	"github.com/juju/cmd/v3"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"

	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/jujuclient"
)

// NewUnregisterCommand returns a command to allow the user to unregister a controller.
func NewUnregisterCommand(store jujuclient.ClientStore) cmd.Command {
	if store == nil {
		panic("valid store must be specified")
	}
	cmd := &unregisterCommand{store: store}
	return modelcmd.WrapBase(cmd)
}

// unregisterCommand removes a Juju controller from the local store.
type unregisterCommand struct {
	modelcmd.CommandBase
	controllerName string
	assumeYes      bool
	store          jujuclient.ClientStore
}

var usageUnregisterDetails = `
Removes local connection information for the specified controller.  This
command does not destroy the controller.  In order to regain access to an
unregistered controller, it will need to be added again using the juju register
command.

Examples:

    juju unregister my-controller

See also:
    destroy-controller
    kill-controller
    register`

// Info implements Command.Info
// `unregister` may seem generic as a command, but aligns with `register`.
func (c *unregisterCommand) Info() *cmd.Info {
	return jujucmd.Info(&cmd.Info{
		Name:    "unregister",
		Args:    "<controller name>",
		Purpose: "Unregisters a Juju controller.",
		Doc:     usageUnregisterDetails,
	})
}

// SetFlags implements Command.SetFlags.
func (c *unregisterCommand) SetFlags(f *gnuflag.FlagSet) {
	f.BoolVar(&c.assumeYes, "y", false, "Do not prompt for confirmation")
	f.BoolVar(&c.assumeYes, "yes", false, "")
	f.BoolVar(&c.assumeYes, "no-prompt", false, "")
}

// SetClientStore implements Command.SetClientStore.
func (c *unregisterCommand) SetClientStore(store jujuclient.ClientStore) {
	c.store = store
}

// Init implements Command.Init.
func (c *unregisterCommand) Init(args []string) error {
	if len(args) < 1 {
		return errors.New("controller name must be specified")
	}
	c.controllerName, args = args[0], args[1:]

	if err := jujuclient.ValidateControllerName(c.controllerName); err != nil {
		return err
	}

	if err := cmd.CheckEmpty(args); err != nil {
		return err
	}
	return nil
}

var unregisterMsg = `
This command will remove connection information for controller %q.
Doing so will prevent you from accessing this controller until
you register it again.

Continue [y/N]?`[1:]

func (c *unregisterCommand) Run(ctx *cmd.Context) error {

	_, err := c.store.ControllerByName(c.controllerName)
	if err != nil {
		return errors.Trace(err)
	}

	if !c.assumeYes {
		fmt.Fprintf(ctx.Stdout, unregisterMsg, c.controllerName)

		if err := jujucmd.UserConfirmYes(ctx); err != nil {
			return errors.Annotate(err, "unregistering controller")
		}
	}

	return (c.store.RemoveController(c.controllerName))
}
