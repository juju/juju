// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package jujuc

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/juju/cmd"
	"launchpad.net/gnuflag"
)

var keyRule = regexp.MustCompile("^[a-z0-9](?:[a-z0-9-]*[a-z0-9])?$")

// ActionSetCommand implements the action-set command.
type ActionSetCommand struct {
	cmd.CommandBase
	ctx  Context
	args [][]string
}

// NewActionSetCommand returns a new ActionSetCommand with the given context.
func NewActionSetCommand(ctx Context) cmd.Command {
	return &ActionSetCommand{ctx: ctx}
}

// Info returns the content for --help.
func (c *ActionSetCommand) Info() *cmd.Info {
	doc := `
action-set adds the given values to the results map of the Action.  This map
is returned to the user after the completion of the Action.

Example usage:
 action-set outfile.size=10G
 action-set foo.bar=2
 action-set foo.baz.val=3
 action-set foo.bar.zab=4
 action-set foo.baz=1

 will yield:

 outfile:
   size: "10G"
 foo:
   bar:
     zab: "4"
   baz: "1"
`
	return &cmd.Info{
		Name:    "action-set",
		Args:    "<key>=<value> [<key>=<value> ...]",
		Purpose: "set action results",
		Doc:     doc,
	}
}

// SetFlags handles known option flags.
func (c *ActionSetCommand) SetFlags(f *gnuflag.FlagSet) {
	// TODO(binary132): add cmd.Input type as in cmd.Output for YAML piping.
}

// Init accepts maps in the form of key=value, key.key2.keyN....=value
func (c *ActionSetCommand) Init(args []string) error {
	c.args = make([][]string, 0)
	for _, arg := range args {
		thisArg := strings.SplitN(arg, "=", 2)
		if len(thisArg) != 2 {
			return fmt.Errorf("argument %q must be of the form key...=value", arg)
		}
		keySlice := strings.Split(thisArg[0], ".")
		// check each key for validity
		for _, key := range keySlice {
			if valid := keyRule.MatchString(key); !valid {
				return fmt.Errorf("key %q must start and end with lowercase alphanumeric, and contain only lowercase alphanumeric and hyphens", key)
			}
		}
		// [key, key, key, key, value]
		c.args = append(c.args, append(keySlice, thisArg[1]))
	}

	return nil
}

// Run adds the given <key list>/<value> pairs, such as foo.bar=baz to the
// existing map of results for the Action.
func (c *ActionSetCommand) Run(ctx *cmd.Context) error {
	for _, argSlice := range c.args {
		valueIndex := len(argSlice) - 1
		keys := argSlice[:valueIndex]
		value := argSlice[valueIndex]
		err := c.ctx.UpdateActionResults(keys, value)
		if err != nil {
			return err
		}
	}

	return nil
}
