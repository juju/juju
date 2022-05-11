// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package config

import (
	"strings"

	"github.com/juju/cmd/v3"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"
	"github.com/juju/utils/keyvalues"
)

// ConfigAction represents the action we want to perform here.
// The action will be set in Init, and then accessed by the child command's
// Run method to decide what to do.
type ConfigAction string

// The strings here are descriptions which will be used in the error message:
// "cannot X and Y simultaneously"
const (
	GetOne  ConfigAction = "get value"
	GetAll  ConfigAction = "get all values"
	Set     ConfigAction = "set key=value pairs"
	SetFile ConfigAction = "use --file flag"
	Reset   ConfigAction = "use --reset flag"
)

// ConfigCommandBase provides a common interface/functionality for configuration
// commands (such as config and model-config). It defines SetFlags and Init
// methods, while the individual command needs to define its own Run and Info
// methods.
type ConfigCommandBase struct {
	// Fields to be set by child
	CantReset []string // keys which can't be reset

	// Flag values
	ConfigFile cmd.FileVar
	reset      []string // Holds the keys to be reset until parsed.

	// Fields to be set during initialisation
	Actions     []ConfigAction // The action which we want to handle, set in Init.
	KeyToGet    string
	KeysToReset []string // Holds keys to be reset after parsing.
	ValsToSet   map[string]string
}

// Info - to be implemented by child command

// SetFlags implements cmd.Command.SetFlags.
func (c *ConfigCommandBase) SetFlags(f *gnuflag.FlagSet) {
	f.Var(&c.ConfigFile, "file", "path to yaml-formatted configuration file")
	f.Var(cmd.NewAppendStringsValue(&c.reset), "reset",
		"Reset the provided comma delimited keys, deletes keys not in the model config")
}

// Init provides a basic implementation of cmd.Command.Init.
// This only parses the actual key/value arguments in the command. Some child
// commands will also need to specify what is being configured (e.g. the app
// name in the case of `juju config`). In this case, the child should define
// its own Init command, where it strips the required arguments and passes the
// rest to the parent.
func (c *ConfigCommandBase) Init(args []string) error {
	// Check if --file has been specified
	if c.ConfigFile.Path != "" {
		c.Actions = append(c.Actions, SetFile)
	}

	// Check if --reset has been specified
	if len(c.reset) != 0 {
		c.Actions = append(c.Actions, Reset)
		err := c.parseResetKeys()
		if err != nil {
			return errors.Trace(err)
		}
	}

	// The remaining arguments are divided into setKeys (args containing `=`)
	// and getKeys, so we can work out what the user is trying to do.
	var setKeys, getKeys []string
	for _, arg := range args {
		splitArg := strings.Split(arg, "=")
		if len(splitArg) > 1 {
			setKeys = append(setKeys, splitArg[0])
		} else {
			getKeys = append(getKeys, arg)
		}
	}

	if len(setKeys) == 0 && len(getKeys) == 0 && len(c.Actions) == 0 {
		// Nothing specified - get full config
		c.Actions = append(c.Actions, GetAll)
	}

	if len(setKeys) == 0 && len(getKeys) == 1 {
		// Get the single key specified
		c.Actions = append(c.Actions, GetOne)
		c.KeyToGet = args[0]
	}

	if len(setKeys) == 0 && len(getKeys) > 1 {
		// Trying to get multiple keys - error
		return errors.New("cannot specify multiple keys to get")
	}

	if len(setKeys) > 0 && len(getKeys) == 0 {
		// Set specified keys to given values
		c.Actions = append(c.Actions, Set)
		var err error
		c.ValsToSet, err = keyvalues.Parse(args, true)
		if err != nil {
			return err
		}
	}

	if len(setKeys) > 0 && len(getKeys) > 0 {
		// Looks like user is trying to get & set simultaneously
		c.Actions = append(c.Actions, GetOne, Set)
		// Error will be return in next step
	}

	// Check that we haven't tried to set/get as well as reset/set from file.
	return c.checkSingleAction()
}

// parseResetKeys splits the keys provided to --reset after trimming any
// leading or trailing comma. It then verifies that we haven't incorrectly
// received any key=value pairs.
func (c *ConfigCommandBase) parseResetKeys() error {
	for _, value := range c.reset {
		keys := strings.Split(strings.Trim(value, ","), ",")
		c.KeysToReset = append(c.KeysToReset, keys...)
	}

	for _, k := range c.KeysToReset {
		if sliceContains(c.CantReset, k) {
			return errors.Errorf("%q cannot be reset", k)
		}
		if strings.Contains(k, "=") {
			return errors.Errorf(
				`--reset accepts a comma delimited set of keys "a,b,c", received: %q`, k)
		}
	}
	return nil
}

// sliceContains is a utility method to check if the given (string) slice
// contains a given value.
func sliceContains(slice []string, val string) bool {
	for _, s := range slice {
		if s == val {
			return true
		}
	}
	return false
}

// checkSingleAction checks that exactly one action has been specified.
func (c *ConfigCommandBase) checkSingleAction() error {
	switch len(c.Actions) {
	case 0:
		return errors.New("no action specified")
	case 1:
		return nil
	default:
		return multiActionError(c.Actions)
	}
}

// multiActionError returns an error saying that the provided actions
// cannot be done simultaneously.
func multiActionError(actions []ConfigAction) error {
	actionList := ""
	for i, descr := range actions {
		// put in the right (grammatical) list separator
		switch i {
		case 0:
			// no separator before list
		case len(actions) - 1:
			actionList += " and "
		default:
			actionList += ", "
		}

		actionList += string(descr)
	}
	return errors.Errorf("cannot %s simultaneously", actionList)
}

// Run - to be implemented by child command
