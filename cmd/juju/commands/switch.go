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
		ReadCurrentController:  modelcmd.ReadCurrentController,
		WriteCurrentController: modelcmd.WriteCurrentController,
	}
	cmd.RefreshModels = cmd.JujuCommandBase.RefreshModels
	return modelcmd.WrapBase(cmd)
}

type switchCommand struct {
	modelcmd.JujuCommandBase
	RefreshModels          func(jujuclient.ClientStore, string, string) error
	ReadCurrentController  func() (string, error)
	WriteCurrentController func(string) error

	Store  jujuclient.ClientStore
	Target string
}

var switchDoc = `
Switch to the specified model or controller.

If the name identifies controller, the client will switch to the
active model for that controller. Otherwise, the name must specify
either the name of a model within the active controller, or a
fully-qualified model with the format "controller:model".
`

func (c *switchCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "switch",
		Args:    "[<controller>|<model>|<controller>:<model>]",
		Purpose: "change the active Juju model",
		Doc:     switchDoc,
	}
}

func (c *switchCommand) Init(args []string) error {
	var err error
	c.Target, err = cmd.ZeroOrOneArgs(args)
	return err
}

func (c *switchCommand) Run(ctx *cmd.Context) (resultErr error) {

	// Get the current name for logging the transition or printing
	// the current controller/model.
	currentControllerName, err := c.ReadCurrentController()
	if err != nil {
		return errors.Trace(err)
	}
	if c.Target == "" {
		currentName, err := c.name(currentControllerName, true)
		if err != nil {
			return errors.Trace(err)
		}
		if currentName == "" {
			return errors.New("no currently specified model")
		}
		fmt.Fprintf(ctx.Stdout, "%s\n", currentName)
		return nil
	}
	currentName, err := c.name(currentControllerName, false)
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
	var newControllerName string
	if newControllerName, err = modelcmd.ResolveControllerName(c.Store, c.Target); err == nil {
		if newControllerName == currentControllerName {
			newName = currentName
			return nil
		} else {
			newName, err = c.name(newControllerName, false)
			if err != nil {
				return errors.Trace(err)
			}
			return errors.Trace(c.WriteCurrentController(newControllerName))
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
		// A controller was specified so see if we should use a local version.
		newControllerName, err = modelcmd.ResolveControllerName(c.Store, newControllerName)
		if err == nil {
			newName = modelcmd.JoinModelName(newControllerName, modelName)
		} else {
			newName = c.Target
		}
	} else {
		if currentControllerName == "" {
			return unknownSwitchTargetError(c.Target)
		}
		newControllerName = currentControllerName
		newName = modelcmd.JoinModelName(newControllerName, modelName)
	}

	accountName, err := c.Store.CurrentAccount(newControllerName)
	if err != nil {
		return errors.Trace(err)
	}
	err = c.Store.SetCurrentModel(newControllerName, accountName, modelName)
	if errors.IsNotFound(err) {
		// The model isn't known locally, so we must query the controller.
		if err := c.RefreshModels(c.Store, newControllerName, accountName); err != nil {
			return errors.Annotate(err, "refreshing models cache")
		}
		err := c.Store.SetCurrentModel(newControllerName, accountName, modelName)
		if errors.IsNotFound(err) {
			return unknownSwitchTargetError(c.Target)
		} else if err != nil {
			return errors.Trace(err)
		}
	} else if err != nil {
		return errors.Trace(err)
	}
	if currentControllerName != newControllerName {
		if err := c.WriteCurrentController(newControllerName); err != nil {
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
func (c *switchCommand) name(controllerName string, machineReadable bool) (string, error) {
	if controllerName == "" {
		return "", nil
	}
	accountName, err := c.Store.CurrentAccount(controllerName)
	if err == nil {
		modelName, err := c.Store.CurrentModel(controllerName, accountName)
		if err == nil {
			return modelcmd.JoinModelName(controllerName, modelName), nil
		} else if !errors.IsNotFound(err) {
			return "", errors.Trace(err)
		}
	} else if !errors.IsNotFound(err) {
		return "", errors.Trace(err)
	}
	// No current account or model.
	if machineReadable {
		return controllerName, nil
	}
	return fmt.Sprintf("%s (controller)", controllerName), nil
}
