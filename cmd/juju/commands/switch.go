// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package commands

import (
	"fmt"
	"os"

	"github.com/juju/cmd/v3"
	"github.com/juju/errors"

	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/juju/common"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/juju/osenv"
	"github.com/juju/juju/jujuclient"
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
	RefreshModels func(jujuclient.ClientStore, string) error

	Store  jujuclient.ClientStore
	Target string
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

func (c *switchCommand) Init(args []string) error {
	var err error
	c.Target, err = cmd.ZeroOrOneArgs(args)
	return err
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
	if errors.IsNotFound(err) {
		currentControllerName = ""
	} else if err != nil {
		return errors.Trace(err)
	}
	if c.Target == "" {
		currentName, err := c.name(store, currentControllerName, true)
		if err != nil {
			return errors.Trace(err)
		}
		if currentName == "" {
			return common.MissingModelNameError("switch")
		}
		fmt.Fprintf(ctx.Stdout, "%s\n", currentName)
		return nil
	}
	currentName, err := c.name(store, currentControllerName, false)
	if err != nil {
		return errors.Trace(err)
	}

	var newName string
	defer func() {
		if resultErr != nil {
			return
		}
		logSwitch(ctx, currentName, &newName)
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

	// If the target identifies a controller, or we want a controller explicitly,
	// then set that as the current controller.
	var newControllerName = c.Target
	var forceController = false
	if c.Target[len(c.Target)-1] == ':' {
		forceController = true
		newControllerName = c.Target[:len(c.Target)-1]
	}
	if _, err = store.ControllerByName(newControllerName); err == nil {
		if newControllerName == currentControllerName {
			newName = currentName
			return nil
		} else {
			newName, err = c.name(store, newControllerName, false)
			if err != nil {
				return errors.Trace(err)
			}
			return errors.Trace(store.SetCurrentController(newControllerName))
		}
	} else if !errors.IsNotFound(err) || forceController {
		return errors.Trace(err)
	}

	// The target is not a controller, so check for a model with
	// the given name. The name can be qualified with the controller
	// name (<controller>:<model>), or unqualified; in the latter
	// case, the model must exist in the current controller.
	newControllerName, modelName := modelcmd.SplitModelName(c.Target)
	if newControllerName != "" {
		if _, err = store.ControllerByName(newControllerName); err != nil {
			return errors.Trace(err)
		}
	} else {
		if currentControllerName == "" {
			return unknownSwitchTargetError(c.Target)
		}
		newControllerName = currentControllerName
	}
	modelName, err = store.QualifiedModelName(newControllerName, modelName)
	if err != nil {
		return errors.Trace(err)
	}
	newName = modelcmd.JoinModelName(newControllerName, modelName)

	err = store.SetCurrentModel(newControllerName, modelName)
	if errors.IsNotFound(err) {
		// The model isn't known locally, so we must query the controller.
		if err := c.RefreshModels(store, newControllerName); err != nil {
			return errors.Annotate(err, "refreshing models cache")
		}
		err := store.SetCurrentModel(newControllerName, modelName)
		if errors.IsNotFound(err) {
			return unknownSwitchTargetError(c.Target)
		} else if err != nil {
			return errors.Trace(err)
		}
	} else if err != nil {
		return errors.Trace(err)
	}
	if currentControllerName != newControllerName {
		if err := store.SetCurrentController(newControllerName); err != nil {
			return errors.Trace(err)
		}
	}
	return nil
}

func unknownSwitchTargetError(name string) error {
	return errors.Errorf("%q is not the name of a model or controller", name)
}

func logSwitch(ctx *cmd.Context, oldName string, newName *string) {
	if *newName == oldName {
		ctx.Infof("%s (no change)", oldName)
	} else {
		ctx.Infof("%s -> %s", oldName, *newName)
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
	if !errors.IsNotFound(err) {
		return "", errors.Trace(err)
	}
	// No current account or model.
	if machineReadable {
		return controllerName, nil
	}
	return fmt.Sprintf("%s (controller)", controllerName), nil
}
