// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package commands

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/juju/errors"
	"github.com/juju/gnuflag"

	"github.com/juju/juju/api/jujuclient"
	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/juju/common"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/internal/cmd"
	"github.com/juju/juju/juju/osenv"
)

func newSwitchCommand() cmd.Command {
	command := &switchCommand{
		Store: jujuclient.NewFileClientStore(),
	}
	command.CanClearCurrentModel = true
	command.RefreshModels = command.CommandBase.RefreshModels
	return modelcmd.WrapBase(command)
}

type switchCommand struct {
	modelcmd.CommandBase
	RefreshModels func(context.Context, jujuclient.ClientStore, string) error

	Store                 jujuclient.ClientStore
	controllerOrModelName string
	modelName             string
	controllerName        string
}

var usageSummary = `
Selects or identifies the current controller and model.`[1:]

var usageDetails = `
When used without an argument, the command shows the current controller
and its active model.

When a single argument without a colon is provided, Juju first looks for a
controller by that name and switches to it and, if it's not found, it tries
to switch to a model within the current controller.

` + "`mycontroller:`" + ` switches to the default model in ` + "`mycontroller`" + `,
` + "`:mymodel`" + ` switches to mymodel in the current controller and
` + "`mycontroller:mymodel`" + ` switches to ` + "`mymodel`" + ` on ` + "`mycontroller`" + `.

The special arguments - (hyphen) instead of a model or a controller allows to return 
to previous model or controller. It can be used as main argument or as flag argument.

The `[1:] + "`juju models`" + ` command can be used to determine the active model
(of any controller). An asterisk denotes it.
`

const usageExamples = `
    juju switch
    juju switch mymodel
    juju switch mycontroller
    juju switch mycontroller:mymodel
    juju switch mycontroller:
    juju switch :mymodel
    juju switch -m mymodel
	juju switch -m mycontroller:mymodel
	juju switch -c mycontroller
    juju switch - # switch to previous controller:model
    juju switch -m - # switch to previous controller on its current model
    juju switch -c - # switch to previous model on the current controller
`

func (c *switchCommand) Info() *cmd.Info {
	return jujucmd.Info(&cmd.Info{
		Name:     "switch",
		Args:     "[<controller>|<model>|<controller>:|:<model>|<controller>:<model>]",
		Purpose:  usageSummary,
		Doc:      usageDetails,
		Examples: usageExamples,
		SeeAlso: []string{
			"controllers",
			"models",
			"show-controller",
		},
	})
}

// SetFlags implements Command.SetFlags.
func (c *switchCommand) SetFlags(f *gnuflag.FlagSet) {
	c.CommandBase.SetFlags(f)
	f.StringVar(&c.modelName, "m", "", "Model to operate in. Accepts [<controller name>:]<model name>")
	f.StringVar(&c.modelName, "model", "", "")
	f.StringVar(&c.controllerName, "c", "", "Controller to operate in")
	f.StringVar(&c.controllerName, "controller", "", "")
}

func (c *switchCommand) Init(args []string) error {
	if c.modelName != "" && c.controllerName != "" {
		return errors.Trace(fmt.Errorf("cannot specify both a --model and --controller"))
	}

	if c.controllerName != "" {
		err := cmd.CheckEmpty(args)
		if err != nil {
			return errors.Trace(fmt.Errorf("no argument accepted when --controller flag  is specified"))
		}
	} else if c.modelName != "" {
		err := cmd.CheckEmpty(args)
		if err != nil {
			return errors.Trace(fmt.Errorf("no argument accepted when --model flag  is specified"))
		}
		// This means we can support arguments like `juju switch -m mycontroller:mymodel`
		c.parseModelName(c.modelName)
	} else {
		target, err := cmd.ZeroOrOneArgs(args)
		if err != nil {
			return errors.Trace(err)
		}
		// if the target does not contain a ":", it is an ambiguous target so
		// we cannot parse it as a model name
		if strings.Contains(target, ":") {
			c.parseModelName(target)
		} else {
			c.controllerOrModelName = target
		}
	}

	// expand "-" syntactic sugar where it's found
	if c.controllerName == "-" {
		previous, _, err := c.Store.PreviousController()
		if err != nil {
			return errors.Annotatef(err, `interpreting "--controller -"`)
		}
		c.controllerName = previous
	}

	if c.modelName == "-" {
		controller, err := c.Store.CurrentController()
		if err != nil {
			return errors.Annotatef(err, `interpreting "--model -"`)
		}
		previous, err := c.Store.PreviousModel(controller)
		if err != nil {
			return errors.Annotatef(err, `interpreting "--model -"`)
		}
		c.modelName = previous
	}

	if c.controllerOrModelName == "-" {
		c.controllerOrModelName = ""

		previousController, changedController, err := c.Store.PreviousController()
		if err != nil && !errors.Is(err, errors.NotFound) {
			return errors.Annotatef(err, `interpreting "-" argument`)
		}

		// if the last switch was intra-controller (i.e. changedController is true), we need
		// to figure out from which model and switch back there. Otherwise (i.e. inter-controller),
		// it is sufficient to switch to just the previous controller.
		if changedController {
			c.controllerName = previousController
		} else {
			c.controllerName, err = c.Store.CurrentController()
			if err != nil {
				return errors.Annotatef(err, `interpreting "-" argument after a model switch`)
			}
			previousModel, err := c.Store.PreviousModel(c.controllerName)
			if errors.Is(err, errors.NotFound) {
				return errors.Errorf(`no previous model for this controller %s, use a qualified switch 'juju switch controller:model' or return to previous controller through 'juju switch -c -' `, c.controllerName)
			} else if err != nil {
				return errors.Annotatef(err, `interpreting "-" argument after a model switch`)
			}
			c.modelName = previousModel
		}
	}

	return nil
}

func (c *switchCommand) parseModelName(name string) {
	parts := strings.SplitN(name, ":", 2)
	if len(parts) == 1 {
		c.modelName = parts[0]
		return
	}
	c.controllerName = parts[0]
	c.modelName = parts[1]
}

// SetClientStore implements Command.SetClientStore.
func (c *switchCommand) SetClientStore(store jujuclient.ClientStore) {
	c.Store = store
}

func (c *switchCommand) Run(ctx *cmd.Context) (resultErr error) {
	store := modelcmd.QualifyingClientStore{
		ClientStore: c.Store,
	}

	// Get the current name for logging the transition or printing
	// the current controller/model.
	currentControllerName, err := modelcmd.DetermineCurrentController(store)
	if errors.Is(err, errors.NotFound) {
		currentControllerName = ""
	} else if err != nil {
		return errors.Trace(err)
	}
	if c.controllerOrModelName == "" && c.modelName == "" && c.controllerName == "" {
		currentName, err := c.name(store, currentControllerName, true)
		if err != nil {
			return errors.Trace(err)
		}
		if currentName == "" {
			return common.MissingModelNameError("switch")
		}
		_, err = fmt.Fprintf(ctx.Stdout, "%s\n", currentName)
		return err
	}
	sourceName, err := c.name(store, currentControllerName, false)
	if err != nil {
		return errors.Trace(err)
	}

	var targetName string
	defer func() {
		if resultErr != nil {
			return
		}
		logSwitch(ctx, sourceName, targetName)
	}()

	// Switch is an alternative way of dealing with models rather than using
	// the JUJU_CONTROLLER or JUJU_MODEL environment settings, and as such,
	// doesn't play too well. If either is set we should report that as the
	// current controller/model, and not allow switching when set.
	if controller := os.Getenv(osenv.JujuControllerEnvKey); controller != "" {
		return errors.Errorf("cannot switch when JUJU_CONTROLLER is overriding the controller (set to %q)", controller)
	}
	if model := os.Getenv(osenv.JujuModelEnvKey); model != "" {
		return errors.Errorf("cannot switch when JUJU_MODEL is overriding the model (set to %q)", model)
	}

	// juju switch something (ambiguous)
	if c.controllerOrModelName != "" {
		// Is it an existing controller ?
		targetName, err = c.trySwitchToController(store, c.controllerOrModelName)
		if err != nil && !errors.Is(err, errors.NotFound) {
			return errors.Annotatef(err, "cannot determine if %q is a valid controller", c.controllerOrModelName)
		}
		if err == nil {
			return // switch successful
		}
		// Is an existing model in current controller ?
		if currentControllerName == "" {
			return errors.Trace(unknownSwitchTargetError(c.controllerOrModelName))
		}
		targetName, err = c.trySwitchToModel(ctx, store, currentControllerName, c.controllerOrModelName)
		if err != nil {
			return errors.Annotatef(err, "cannot determine if %q is a valid model", c.controllerOrModelName)
		}
		return
	}

	// Juju switch non ambiguous
	if c.modelName == "" {
		targetName, err = c.trySwitchToController(store, c.controllerName)
		if err != nil {
			return errors.Annotate(err, "invalid target controller")
		}
		return
	}

	if c.controllerName == "" {
		if currentControllerName == "" {
			return errors.Trace(unknownSwitchTargetError(c.modelName))
		}
		c.controllerName = currentControllerName
	}
	targetName, err = c.trySwitchToModel(ctx, store, c.controllerName, c.modelName)
	if err != nil {
		return errors.Annotate(err, "invalid target model")
	}
	return
}

func unknownSwitchTargetError(name string) error {
	return errors.Errorf("%q is not the name of a model or controller", name)
}

func logSwitch(ctx *cmd.Context, oldName string, newName string) {
	if newName == oldName {
		ctx.Infof("%s (no change)", oldName)
	} else {
		ctx.Infof("%s -> %s", oldName, newName)
	}
}

// name returns the name of the current model for the specified controller
// if one is set, otherwise the controller name with an indicator that it
// is the name of a controller and not a model.
func (c *switchCommand) name(store jujuclient.ModelGetter, controllerName string, machineReadable bool) (string, error) {
	if controllerName == "" {
		return "", nil
	}
	modelName, err := store.CurrentModel(controllerName)
	if err == nil {
		return modelcmd.JoinModelName(controllerName, modelName), nil
	}
	if !errors.Is(err, errors.NotFound) {
		return "", errors.Trace(err)
	}
	// No current account or model.
	if machineReadable {
		return controllerName, nil
	}
	return fmt.Sprintf("%s (controller)", controllerName), nil
}

func (c *switchCommand) trySwitchToController(store jujuclient.ClientStore, controller string) (string, error) {
	// Check that the controller actually exists
	_, err := store.ControllerByName(controller)
	if err != nil {
		// If something get wrong
		return "", errors.Trace(err)
	}
	targetName, err := c.name(store, controller, false)
	if err != nil {
		return "", errors.Trace(err)
	}
	return targetName, errors.Trace(store.SetCurrentController(controller))
}

func (c *switchCommand) trySwitchToModel(ctx context.Context, store modelcmd.QualifyingClientStore, controller string, model string) (string, error) {
	if err := store.SetCurrentController(controller); err != nil {
		return "", errors.Trace(err)
	}
	modelName, err := store.QualifiedModelName(controller, model)
	if err != nil {
		return "", errors.Trace(err)
	}

	err = store.SetCurrentModel(controller, modelName)
	if errors.Is(err, errors.NotFound) {
		// The model isn't known locally, so we must query the controller.
		if err := c.RefreshModels(ctx, store, controller); err != nil {
			return "", errors.Annotate(err, "refreshing models cache")
		}
		err := store.SetCurrentModel(controller, modelName)
		if errors.Is(err, errors.NotFound) {
			return "", unknownSwitchTargetError(controller + ":" + model)
		} else if err != nil {
			return "", errors.Trace(err)
		}
	} else if err != nil {
		return "", errors.Trace(err)
	}
	return modelcmd.JoinModelName(controller, modelName), nil
}
