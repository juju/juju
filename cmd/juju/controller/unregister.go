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
	modelcmd.DestroyConfirmationCommandBase

	controllerName string
	store          jujuclient.ClientStore
}

var usageUnregisterDetails = `
Removes local connection information for the specified controller.

This command does not destroy the controller.  In order to regain access to an
unregistered controller, it will need to be added again using the ` + "`juju register`" + `
command.

`

const usageUnregisterExamples = `
    juju unregister my-controller
`

// Info implements Command.Info
// `unregister` may seem generic as a command, but aligns with `register`.
func (c *unregisterCommand) Info() *cmd.Info {
	return jujucmd.Info(&cmd.Info{
		Name:     "unregister",
		Args:     "<controller name>",
		Purpose:  "Unregisters a Juju controller.",
		Doc:      usageUnregisterDetails,
		Examples: usageUnregisterExamples,
		SeeAlso: []string{
			"destroy-controller",
			"kill-controller",
			"register",
		},
	})
}

// SetFlags implements Command.SetFlags.
func (c *unregisterCommand) SetFlags(f *gnuflag.FlagSet) {
	c.DestroyConfirmationCommandBase.SetFlags(f)
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
		return errors.Trace(err)
	}

	return nil
}

var unregisterMsg = `
This command will remove connection information for controller %q.
The controller will not be destroyed but you must register it again
if you want to access it.
`[1:]

func (c *unregisterCommand) Run(ctx *cmd.Context) error {

	_, err := c.store.ControllerByName(c.controllerName)
	if err != nil {
		return errors.Trace(err)
	}

	if c.DestroyConfirmationCommandBase.NeedsConfirmation() {
		fmt.Fprintf(ctx.Stderr, unregisterMsg, c.controllerName)
		if err := jujucmd.UserConfirmName(c.controllerName, "controller", ctx); err != nil {
			return errors.Annotate(err, "unregistering controller")
		}
	}

	return (c.store.RemoveController(c.controllerName))
}
