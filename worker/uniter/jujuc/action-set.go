// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package jujuc

import (
	"fmt"
	"strings"

	"github.com/juju/cmd"
	"launchpad.net/gnuflag"
)

type nestedMap map[string]interface{}

// ActionSetCommand implements the action-set command.
type ActionSetCommand struct {
	cmd.CommandBase
	ctx  Context
	args map[string]string
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
 action-set foo.bar.baz=2 foo.bar.zab=3
 action-set foo.bar.baz=4

 will yield:

 outfile:
   size: "10G"
 foo:
   bar:
     baz: "4"
     zab: "3"
`
	return &cmd.Info{
		Name:    "action-set",
		Args:    "<key>=<value> [<key>.<key>....=<value> ...]",
		Purpose: "set action results",
		Doc:     doc,
	}
}

// SetFlags handles known option flags.
// TODO(binary132): add cmd.Input type as in cmd.Output for YAML piping.
func (c *ActionSetCommand) SetFlags(f *gnuflag.FlagSet) {
}

// Init accepts maps in the form of key=value, key.key2.keyN....=value
func (c *ActionSetCommand) Init(args []string) error {
	c.args = make(map[string]string)
	for _, arg := range args {
		thisArg := strings.SplitN(arg, "=", 2)
		if len(thisArg) != 2 {
			return fmt.Errorf("argument %q must be of the form key...=value", arg)
		}
		c.args[thisArg[0]] = thisArg[1]
	}

	return nil
}

// Run adds the given phrases (such as foo.bar=baz) to the existing map of
// results for the Action.
func (c *ActionSetCommand) Run(ctx *cmd.Context) error {
	if len(c.args) == 0 {
		return nil
	}

	// c.ctx.ActionResults() will get us the current map of
	// action results, allowing us to add to it.  We don't care if it's
	// already failed, so the user doesn't lose useful debugging info.
	results, _ := c.ctx.ActionResults()
	nm := nestedMap(results)

	for keys, value := range c.args {
		keySlice := strings.Split(keys, ".")
		nm.addValueToMap(keySlice, value)
	}

	c.ctx.ActionSetResults(map[string]interface{}(nm))
	return nil
}

// addValueToMap adds the given value to the map on which the method is run.
// This allows us to merge maps such as {foo: {bar: baz}} and {foo: {baz: faz}}
// into {foo: {bar: baz, baz: faz}}.
func (nm nestedMap) addValueToMap(keys []string, value string) {
	next := nm

	for i := range keys {
		// if we are on last key set the value.
		// shouldn't be a problem.  overwrites existing vals.
		if i == len(keys)-1 {
			next[keys[i]] = value
			break
		}

		if iface, ok := next[keys[i]]; ok {
			switch typed := iface.(type) {
			case map[string]interface{}:
				// If we already had a map inside, keep
				// stepping through.
				next = typed
			default:
				// If we didn't, then overwrite value
				// with a map and iterate with that.
				m := map[string]interface{}{}
				next[keys[i]] = m
				next = m
			}
		} else {
			// Otherwise, it wasn't present, so make it and step
			// into.
			m := map[string]interface{}{}
			next[keys[i]] = m
			next = m
		}
	}
}
