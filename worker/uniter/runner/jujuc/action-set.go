// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package jujuc

import (
	"fmt"
	"strings"

	"github.com/juju/charm/v12"
	"github.com/juju/cmd/v3"
	"github.com/juju/gnuflag"

	jujucmd "github.com/juju/juju/cmd"
)

var keyRule = charm.GetActionNameRule()

// list of action result keys that are not allowed to be set by users
var reservedKeys = []string{"stdout", "stdout-encoding", "stderr", "stderr-encoding"}

// ActionSetCommand implements the action-set command.
type ActionSetCommand struct {
	cmd.CommandBase
	ctx  Context
	args [][]string
}

// NewActionSetCommand returns a new ActionSetCommand with the given context.
func NewActionSetCommand(ctx Context) (cmd.Command, error) {
	return &ActionSetCommand{ctx: ctx}, nil
}

// Info returns the content for --help.
func (c *ActionSetCommand) Info() *cmd.Info {
	reservedText := `"` + strings.Join(reservedKeys, `", "`) + `"`
	doc := fmt.Sprintf(`
action-set adds the given values to the results map of the action. This map
is returned to the user after the completion of the action.
Keys must be given as a flat period-separated path of keys.
Each key must start and end with lowercase alphanumeric,
and contain only lowercase alphanumeric and hyphens.
Examples of valid key paths:
["foo", "500", "5-o-0", "foo.bar", "foo.bar.baz", "foo-bar.baz"]
Examples of invalid key paths:
["-foo", "foo-", "foo-.bar", "foo!bar", "foo..bar", ".foo", "foo.", ".", ""]
The following special keys are reserved for internal use, and thus not allowed:
%s.
Values are always interpreted as strings.
The final result will be a nested object containing the merged results,
with any conflicting values overwriting previous values.

`, reservedText)
	examples := `
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
	return jujucmd.Info(&cmd.Info{
		Name:     "action-set",
		Args:     "<key-path>=<value> [<key-path>=<value> ...]",
		Purpose:  "Set action results.",
		Doc:      doc,
		Examples: examples,
	})
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

			for _, reserved := range reservedKeys {
				if reserved == key {
					return fmt.Errorf(`cannot set reserved action key "%s"`, key)
				}
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
