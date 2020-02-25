// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package space

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"
	"gopkg.in/juju/names.v3"

	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/core/network"
)

// NewRemoveCommand returns a command used to remove a space.
func NewRemoveCommand() modelcmd.ModelCommand {
	return modelcmd.Wrap(&RemoveCommand{})
}

// RemoveCommand calls the API to remove an existing network space.
type RemoveCommand struct {
	SpaceCommandBase
	name string

	assumeYes bool
	force     bool
}

const removeCommandDoc = `
Removes an existing Juju network space with the given name. Any subnets
associated with the space will be transferred to the default space.
The command will fail if existing constraints, bindings or controller settings are bound to the given space.

If the --force option is specified, the space will be deleted even if there are existing bindings, constraints or settings.

Examples:

Remove a space by name:
	juju remove-space db-space

Remove a space by name with force, without need for confirmation:
	juju remove-space db-space --force -y

See also:
	add-space
	list-spaces
	reload-spaces
	rename-space
	show-space
`

var removeSpaceMsgNoBounds = `
WARNING! This command will remove the space. 
Safe removal possible. No constraints, bindings or controller config found with dependency on the given space.

Continue [y/N]? `[1:]

// Info is defined on the cmd.Command interface.
func (c *RemoveCommand) Info() *cmd.Info {
	return jujucmd.Info(&cmd.Info{
		Name:    "remove-space",
		Args:    "<name>",
		Purpose: "Remove a network space.",
		Doc:     strings.TrimSpace(removeCommandDoc),
	})
}

// SetFlags implements Command.SetFlags.
func (c *RemoveCommand) SetFlags(f *gnuflag.FlagSet) {
	c.SpaceCommandBase.SetFlags(f)
	f.BoolVar(&c.force, "force", false, "remove the offer as well as any relations to the offer")
	f.BoolVar(&c.assumeYes, "y", false, "Do not prompt for confirmation")
	f.BoolVar(&c.assumeYes, "yes", false, "")
}

// Init is defined on the cmd.Command interface. It checks the
// arguments for sanity and sets up the command to run.
func (c *RemoveCommand) Init(args []string) (err error) {
	defer errors.DeferredAnnotatef(&err, "invalid arguments specified")

	// Validate given name.
	if len(args) == 0 {
		return errors.New("space name is required")
	}
	givenName := args[0]
	if !names.IsValidSpace(givenName) {
		return errors.Errorf("%q is not a valid space name", givenName)
	}
	c.name = givenName

	return cmd.CheckEmpty(args[1:])
}

// Run implements Command.Run.
func (c *RemoveCommand) Run(ctx *cmd.Context) error {
	controllerName, err := c.ControllerName()
	if err != nil {
		return errors.Trace(err)
	}
	store := c.ClientStore()
	currentModel, err := store.CurrentModel(controllerName)
	if err != nil {
		return errors.Trace(err)
	}
	return c.RunWithAPI(ctx, func(api SpaceAPI, ctx *cmd.Context) error {
		if !c.assumeYes && c.force {
			if err := c.handleForceOption(api, currentModel, ctx); err != nil {
				return errors.Annotatef(err, "cannot remove space %q", c.name)
			}
		}
		if bounds, err := api.RemoveSpace(c.name, c.force, false); err != nil || buildRemoveErrorMsg(bounds, currentModel) != nil {
			return errors.Annotatef(err, "cannot remove space %q", c.name)
		}
		ctx.Infof("removed space %q", c.name)
		return nil
	})
}

func (c *RemoveCommand) handleForceOption(api SpaceAPI, currentModel string, ctx *cmd.Context) error {
	bounds, err := api.RemoveSpace(c.name, false, true)
	if err != nil {
		return err
	}
	if buildRemoveErrorMsg(bounds, currentModel) == nil {
		fmt.Fprintf(ctx.Stdout, removeSpaceMsgNoBounds)
	} else {
		removeSpaceMsg := fmt.Sprintf(""+
			"WARNING! This command will remove the space"+
			" with the following existing boundaries:"+
			"\n\n%v\n\n\n"+
			"Continue [y/N]?", buildRemoveErrorMsg(bounds, currentModel))
		fmt.Fprintf(ctx.Stdout, removeSpaceMsg)
	}
	if err := jujucmd.UserConfirmYes(ctx); err != nil {
		return errors.Annotate(err, "space removal")
	}
	return nil
}

func buildRemoveErrorMsg(removeSpace network.RemoveSpace, currentModel string) error {
	var errMsg bytes.Buffer
	constraints := removeSpace.Constraints
	bindings := removeSpace.Bindings
	config := removeSpace.ControllerConfig
	spaceName := removeSpace.Space

	if len(removeSpace.Constraints) > 0 {
		fmt.Fprintf(&errMsg, "\n- %q is used as a "+
			"constraint on: %v", spaceName, strings.Join(constraints, ", "))

		if removeSpace.HasModelConstraint {
			fmt.Fprintf(&errMsg, "\n- %q is used as a "+
				"model constraint: %v", spaceName, currentModel)
		}
	}
	if len(removeSpace.Bindings) > 0 {
		fmt.Fprintf(&errMsg, "\n- %q is used as a "+
			"binding on: %v", spaceName, strings.Join(bindings, ", "))
	}
	if len(removeSpace.ControllerConfig) > 0 {
		fmt.Fprintf(&errMsg, "\n- %q is used for controller "+
			"config(s): %v", spaceName, strings.Join(config, ", "))
	}

	if errMsg.String() == "" {
		return nil
	}
	return errors.New(strings.Trim(errMsg.String(), "[]"))
}
