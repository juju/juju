// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package space

import (
	"fmt"
	"strings"

	"github.com/juju/cmd/v3"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"
	"github.com/juju/names/v5"

	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/rpc/params"
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
The command will fail if existing constraints, bindings or controller settings
are bound to the given space.

If the ` + "`--force`" + ` option is specified, the space will be deleted even
if there are existing bindings, constraints or settings.

`

const removeCommandExamples = `
Remove a space by name:

	juju remove-space db-space

Remove a space by name with force, without need for confirmation:

	juju remove-space db-space --force -y
`

var removeSpaceMsgNoBounds = `
WARNING! This command will remove the space.
Safe removal possible. No constraints, bindings or controller config found with dependency on the given space.
`[1:]

var removeSpaceMsgBounds = `
WARNING! This command will remove the space with the following existing boundaries:

%v
`[1:]

// Info is defined on the cmd.Command interface.
func (c *RemoveCommand) Info() *cmd.Info {
	return jujucmd.Info(&cmd.Info{
		Name:     "remove-space",
		Args:     "<name>",
		Purpose:  "Remove a network space.",
		Doc:      strings.TrimSpace(removeCommandDoc),
		Examples: removeCommandExamples,
		SeeAlso: []string{
			"add-space",
			"spaces",
			"reload-spaces",
			"rename-space",
			"show-space",
		},
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

	return c.RunWithSpaceAPI(ctx, func(api SpaceAPI, ctx *cmd.Context) error {
		if !c.assumeYes && c.force {
			if err := c.handleForceOption(api, currentModel, ctx); err != nil {
				return errors.Annotatef(err, "cannot remove space %q", c.name)
			}
		}

		space, err := api.RemoveSpace(c.name, c.force, false)
		if err != nil {
			return errors.Annotatef(err, "cannot remove space %q", c.name)
		}

		result, err := removeSpaceFromResult(c.name, space)
		if err != nil {
			return errors.Annotatef(err, "failed to parse space %q", c.name)
		}
		errorList := buildRemoveErrorList(result, currentModel)
		if len(errorList) > 0 {
			return errors.Errorf("Cannot remove space %q\n\n%s\n\nUse --force to remove space\n", c.name, strings.Join(errorList, "\n"))
		}

		ctx.Infof("removed space %q", c.name)
		return nil
	})
}

func (c *RemoveCommand) handleForceOption(api SpaceAPI, currentModel string, ctx *cmd.Context) error {
	space, err := api.RemoveSpace(c.name, false, true)
	if err != nil {
		return errors.Trace(err)
	}

	result, err := removeSpaceFromResult(c.name, space)
	if err != nil {
		return errors.Annotatef(err, "failed to parse space %q", c.name)
	}

	errorList := buildRemoveErrorList(result, currentModel)
	if len(errorList) == 0 {
		fmt.Fprint(ctx.Stderr, removeSpaceMsgNoBounds)
	} else {
		fmt.Fprintf(ctx.Stderr, removeSpaceMsgBounds, strings.Join(errorList, "\n"))
	}
	if err := jujucmd.UserConfirmYes(ctx); err != nil {
		return errors.Annotate(err, "space removal")
	}
	return nil
}

func removeSpaceFromResult(name string, result params.RemoveSpaceResult) (RemoveSpace, error) {
	constraints, err := convertEntitiesToStringAndSkipModel(result.Constraints)
	if err != nil {
		return RemoveSpace{}, err
	}
	hasModel, err := hasModelConstraint(result.Constraints)
	if err != nil {
		return RemoveSpace{}, err
	}
	bindings, err := convertEntitiesToStringAndSkipModel(result.Bindings)
	if err != nil {
		return RemoveSpace{}, err
	}

	return RemoveSpace{
		HasModelConstraint: hasModel,
		Space:              name,
		Constraints:        constraints,
		Bindings:           bindings,
		ControllerConfig:   result.ControllerSettings,
	}, nil
}

func buildRemoveErrorList(removeSpace RemoveSpace, currentModel string) []string {
	constraints := removeSpace.Constraints
	bindings := removeSpace.Bindings
	config := removeSpace.ControllerConfig
	spaceName := removeSpace.Space

	var list []string
	var msg string
	if len(removeSpace.Constraints) > 0 {
		msg = "- %q is used as a constraint on: %v"
		list = append(list, fmt.Sprintf(msg, spaceName, strings.Join(constraints, ", ")))

		if removeSpace.HasModelConstraint {
			msg = "- %q is used as a model constraint: %v"
			list = append(list, fmt.Sprintf(msg, spaceName, currentModel))
		}
	}
	if len(removeSpace.Bindings) > 0 {
		msg = "- %q is used as a binding on: %v"
		list = append(list, fmt.Sprintf(msg, spaceName, strings.Join(bindings, ", ")))
	}
	if len(removeSpace.ControllerConfig) > 0 {
		msg = "- %q is used for controller config(s): %v"
		list = append(list, fmt.Sprintf(msg, spaceName, strings.Join(config, ", ")))
	}

	return list
}

// RemoveSpace represents space information why a space could not be removed.
type RemoveSpace struct {
	// The space which cannot be removed. Only with --force.
	Space string `json:"space" yaml:"space"`
	// HasModelConstraint is the model constraint.
	HasModelConstraint bool `json:"has-model-constraint" yaml:"has-model-constraint"`
	// Constraints are the constraints which blocks the remove. Blocking Constraints are: Application.
	Constraints []string `json:"constraints,omitempty" yaml:"constraints,omitempty"`
	// Bindings are the application bindings which blocks the remove.
	Bindings []string `json:"bindings,omitempty" yaml:"bindings,omitempty"`
	// ControllerConfig are the config settings of the controller model which are using the space.
	// This is only valid if the current model is a controller model.
	ControllerConfig []string `json:"controller-settings,omitempty" yaml:"controller-settings,omitempty"`
}
