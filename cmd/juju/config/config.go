// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package config

import (
	"fmt"
	"io"
	"strings"

	"github.com/juju/cmd/v3"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"
	"github.com/juju/utils/v3/keyvalues"
	"gopkg.in/yaml.v3"
)

// Action represents the action we want to perform here.
// The action will be set in Init, and then accessed by the child command's
// Run method to decide what to do.
type Action string

// The strings here are descriptions which will be used in the error message:
// "cannot X and Y simultaneously"
const (
	GetOne  Action = "get value"
	GetAll  Action = "get all values"
	SetArgs Action = "set key=value pairs"
	SetFile Action = "use --file flag"
	Reset   Action = "use --reset flag"
)

// Attrs represents configuration attributes from either the command-line
// (key=value arguments) or a yaml file.
type Attrs map[string]interface{}

// ConfigCommandBase provides a common interface/functionality for configuration
// commands (such as config and model-config). It defines SetFlags and Init
// methods, while the individual command needs to define its own Run and Info
// methods.
type ConfigCommandBase struct {
	// Fields to be set by child
	Resettable bool     // does this command allow resetting config values?
	CantReset  []string // keys which can't be reset

	// Flag values
	ConfigFile cmd.FileVar
	reset      []string // Holds the keys to be reset until parsed.
	Color      bool
	NoColor    bool

	// Fields to be set during initialisation
	Actions     []Action // The action which we want to handle, set in Init.
	KeysToGet   []string
	KeysToReset []string // Holds keys to be reset after parsing.
	ValsToSet   Attrs
}

// Info - to be implemented by child command

// SetFlags implements cmd.Command.SetFlags.
func (c *ConfigCommandBase) SetFlags(f *gnuflag.FlagSet) {
	f.Var(&c.ConfigFile, "file", "Path to yaml-formatted configuration file")
	f.BoolVar(&c.Color, "color", false, "Use ANSI color codes in output")
	f.BoolVar(&c.NoColor, "no-color", false, "Disable ANSI color codes in tabular output")
	if c.Resettable {
		f.Var(cmd.NewAppendStringsValue(&c.reset), "reset",
			"Reset the provided comma delimited keys")
	}

}

// Init provides a basic implementation of cmd.Command.Init.
// This only parses the actual key/value arguments in the command. Some child
// commands will also need to specify what is being configured (e.g. the app
// name in the case of `juju config`). In this case, the child should define
// its own Init command, where it strips the required arguments and passes the
// rest to the parent.
func (c *ConfigCommandBase) Init(args []string) error {
	// Don't change the order - it's important SetFile comes before Set/Reset
	// so that set/reset arguments override args specified in file

	// Check if --file has been specified
	if c.ConfigFile.Path != "" {
		c.Actions = append(c.Actions, SetFile)
	}

	// Check if --reset has been specified
	if c.Resettable && len(c.reset) != 0 {
		err := c.parseResetKeys()
		if err != nil {
			return errors.Trace(err)
		}
		c.Actions = append(c.Actions, Reset)
	}

	// The remaining arguments are divided into keys to set (if the arg
	// contains `=`) and keys to get (otherwise).
	c.ValsToSet = make(Attrs)
	for _, arg := range args {
		splitArg := strings.SplitN(arg, "=", 2)
		if len(splitArg) == 2 {
			key := splitArg[0]
			if len(key) == 0 {
				return errors.Errorf(`expected "key=value", got %q`, arg)
			}
			if _, exists := c.ValsToSet[key]; exists {
				return keyvalues.DuplicateError(
					fmt.Sprintf("key %q specified more than once", key))
			}
			c.ValsToSet[key] = splitArg[1]
		} else {
			c.KeysToGet = append(c.KeysToGet, arg)
		}
	}

	if len(c.KeysToGet) > 0 {
		c.Actions = append(c.Actions, GetOne)
	}
	if len(c.ValsToSet) > 0 {
		c.Actions = append(c.Actions, SetArgs)
	}
	if len(c.Actions) == 0 {
		// Nothing has been specified - so we get all
		c.Actions = []Action{GetAll}
		return nil
	}

	// Check the requested combination of actions is valid
	return c.validateActions()
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
func sliceContains[T comparable](slice []T, val T) bool {
	for _, s := range slice {
		if s == val {
			return true
		}
	}
	return false
}

// validateActions checks that the requested combination of actions is valid
// (i.e. we permit doing these actions simultaneously). The rules are:
//   - Set & Reset can be done simultaneously, as long as we don't try to
//     set and reset the same key;
//   - SetFile can be done simultaneously with Set and/or Reset - in this case,
//     the Set/Reset arguments will override anything specified in the file;
//   - GetOne cannot be done along with any other action.
func (c *ConfigCommandBase) validateActions() error {
	if len(c.Actions) > 1 && sliceContains(c.Actions, GetOne) {
		// We have specified GetOne and another action - invalid
		return multiActionError(c.Actions)
	}

	// This combination of actions seems valid
	// Check we're not trying to get multiple keys
	if len(c.KeysToGet) > 1 {
		return errors.New("cannot specify multiple keys to get")
	}

	// Check there are no keys being set/reset
	for _, key := range c.KeysToReset {
		if _, exists := c.ValsToSet[key]; exists {
			return errors.Errorf("cannot set and reset key %q simultaneously", key)
		}
	}
	return nil
}

// multiActionError returns an error saying that the provided actions
// cannot be done simultaneously.
func multiActionError(actions []Action) error {
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

// Helper methods - to be used by child commands

// ReadFile reads the yaml file at c.ConfigFile.Path, and parses it into
// an Attrs object.
func (c *ConfigCommandBase) ReadFile(ctx *cmd.Context) (Attrs, error) {
	var (
		data []byte
		err  error
	)
	if c.ConfigFile.Path == "-" {
		// Read from stdin
		data, err = io.ReadAll(ctx.Stdin)
	} else {
		// Read from file
		data, err = c.ConfigFile.Read(ctx)
	}
	if err != nil {
		return nil, errors.Trace(err)
	}

	attrs := make(Attrs)
	if err := yaml.Unmarshal(data, &attrs); err != nil {
		return nil, errors.Trace(err)
	}
	return attrs, nil
}
