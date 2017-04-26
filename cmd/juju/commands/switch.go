// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package commands

import (
	"fmt"
	"os"

	"github.com/juju/cmd"
	"github.com/juju/errors"

	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/juju/osenv"
	"github.com/juju/juju/jujuclient"
)

func newSwitchCommand() cmd.Command {
	cmd := &switchCommand{
		Store: jujuclient.NewFileClientStore(),
	}
	cmd.RefreshModels = cmd.CommandBase.RefreshModels
	return modelcmd.WrapBase(cmd)
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
When switching by controller name alone, the model
you get is the active model for that controller. If you want a different
model then you must switch using controller:model notation or switch to 
the controller and then to the model. 
The `[1:] + "`juju models`" + ` command can be used to determine the active model
(of any controller). An asterisk denotes it.

Examples:
    juju switch
    juju switch mymodel
    juju switch mycontroller
    juju switch mycontroller:mymodel

See also: 
    controllers
    models
    show-controller`

func (c *switchCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "switch",
		Args:    "[<controller>|<model>|<controller>:<model>]",
		Purpose: usageSummary,
		Doc:     usageDetails,
	}
}

func (c *switchCommand) Init(args []string) error {
	var err error
	c.Target, err = cmd.ZeroOrOneArgs(args)
	return err
}

func (c *switchCommand) Run(ctx *cmd.Context) (resultErr error) {
	store := modelcmd.QualifyingClientStore{c.Store}

	// Get the current name for logging the transition or printing
	// the current controller/model.
	currentControllerName, err := store.CurrentController()
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
			return errors.New("no currently specified model")
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

	// Switch is an alternative way of dealing with environments than using
	// the JUJU_MODEL environment setting, and as such, doesn't play too well.
	// If JUJU_MODEL is set we should report that as the current environment,
	// and not allow switching when it is set.
	if model := os.Getenv(osenv.JujuModelEnvKey); model != "" {
		return errors.Errorf("cannot switch when JUJU_MODEL is overriding the model (set to %q)", model)
	}

	// If the target identifies a controller, then set that as the current controller.
	var newControllerName = c.Target
	if _, err = store.ControllerByName(c.Target); err == nil {
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
	} else if !errors.IsNotFound(err) {
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
