// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package commands

import (
	"fmt"
	"os"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/utils/set"
	"launchpad.net/gnuflag"

	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/environs/configstore"
)

func newSwitchCommand() cmd.Command {
	return &switchCommand{}
}

type switchCommand struct {
	cmd.CommandBase
	ModelName string
	List      bool
}

var switchDoc = `
Show or change the default juju model or controller name.

If no command line parameters are passed, switch will output the current
model as defined by the file $JUJU_DATA/current-model.

If a command line parameter is passed in, that value will is stored in the
current model file if it represents a valid model name.
`

const controllerSuffix = " (controller)"

func (c *switchCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "switch",
		Args:    "[model name]",
		Purpose: "show or change the default juju model or controller name",
		Doc:     switchDoc,
	}
}

func (c *switchCommand) SetFlags(f *gnuflag.FlagSet) {
	f.BoolVar(&c.List, "l", false, "list the model names")
	f.BoolVar(&c.List, "list", false, "")
}

func (c *switchCommand) Init(args []string) (err error) {
	c.ModelName, err = cmd.ZeroOrOneArgs(args)
	return
}

func getConfigstoreOptions() (set.Strings, set.Strings, error) {
	store, err := configstore.Default()
	if err != nil {
		return nil, nil, errors.Annotate(err, "failed to get config store")
	}
	environmentNames, err := store.List()
	if err != nil {
		return nil, nil, errors.Annotate(err, "failed to list models in config store")
	}
	controllerNames, err := store.ListSystems()
	if err != nil {
		return nil, nil, errors.Annotate(err, "failed to list controllers in config store")
	}
	// Also include the controllers.
	return set.NewStrings(environmentNames...), set.NewStrings(controllerNames...), nil
}

func (c *switchCommand) Run(ctx *cmd.Context) error {
	// Switch is an alternative way of dealing with environments than using
	// the JUJU_MODEL environment setting, and as such, doesn't play too well.
	// If JUJU_MODEL is set we should report that as the current environment,
	// and not allow switching when it is set.

	// Passing through the empty string reads the default environments.yaml file.
	// If the environments.yaml file doesn't exist, just list environments in
	// the configstore.
	configEnvirons, configControllers, err := getConfigstoreOptions()
	if err != nil {
		return err
	}
	names := set.NewStrings()
	names = names.Union(configEnvirons)
	names = names.Union(configControllers)

	if c.List {
		// List all environments and controllers.
		if c.ModelName != "" {
			return errors.New("cannot switch and list at the same time")
		}
		for _, name := range names.SortedValues() {
			if configControllers.Contains(name) && !configEnvirons.Contains(name) {
				name += controllerSuffix
			}
			fmt.Fprintf(ctx.Stdout, "%s\n", name)
		}
		return nil
	}

	jujuEnv := os.Getenv("JUJU_MODEL")
	if jujuEnv != "" {
		if c.ModelName == "" {
			fmt.Fprintf(ctx.Stdout, "%s\n", jujuEnv)
			return nil
		} else {
			return errors.Errorf("cannot switch when JUJU_MODEL is overriding the model (set to %q)", jujuEnv)
		}
	}

	current, isController, err := modelcmd.CurrentConnectionName()
	if err != nil {
		return errors.Trace(err)
	}
	if current != "" && isController {
		current += controllerSuffix
	}

	// Handle the different operation modes.
	switch {
	case c.ModelName == "" && current == "":
		// Nothing specified and nothing to switch to.
		return errors.New("no currently specified model")
	case c.ModelName == "":
		// Simply print the current environment.
		fmt.Fprintf(ctx.Stdout, "%s\n", current)
		return nil
	default:
		// Switch the environment.
		if !names.Contains(c.ModelName) {
			return errors.Errorf("%q is not a name of an existing defined model or controller", c.ModelName)
		}
		// If the name is not in the environment set, but is in the controller
		// set, then write the name into the current controller file.
		logger.Debugf("controllers: %v", configControllers)
		logger.Debugf("models: %v", configEnvirons)
		newEnv := c.ModelName
		if configControllers.Contains(newEnv) && !configEnvirons.Contains(newEnv) {
			return modelcmd.SetCurrentController(ctx, newEnv)
		}
		return modelcmd.SetCurrentModel(ctx, newEnv)
	}
}
