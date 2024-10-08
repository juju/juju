// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package jujuc

import (
	"strings"

	"github.com/juju/cmd/v4"
	"github.com/juju/gnuflag"

	jujucmd "github.com/juju/juju/cmd"
)

// ActionGetCommand implements the action-get command.
type ActionGetCommand struct {
	cmd.CommandBase
	ctx  Context
	keys []string
	out  cmd.Output
}

// NewActionGetCommand returns an ActionGetCommand for use with the given
// context.
func NewActionGetCommand(ctx Context) (cmd.Command, error) {
	return &ActionGetCommand{ctx: ctx}, nil
}

// Info returns the content for --help.
func (c *ActionGetCommand) Info() *cmd.Info {
	doc := `
action-get will print the value of the parameter at the given key, serialized
as YAML.  If multiple keys are passed, action-get will recurse into the param
map as needed.
`
	examples := `
    TIMEOUT=$(action-get timeout)
`
	return jujucmd.Info(&cmd.Info{
		Name:     "action-get",
		Args:     "[<key>[.<key>.<key>...]]",
		Purpose:  "Get action parameters.",
		Doc:      doc,
		Examples: examples,
	})
}

// SetFlags handles known option flags; in this case, [--output={json|yaml}]
// and --help.
func (c *ActionGetCommand) SetFlags(f *gnuflag.FlagSet) {
	c.out.AddFlags(f, "smart", cmd.DefaultFormatters.Formatters())
}

// Init makes sure there are no additional unknown arguments to action-get.
func (c *ActionGetCommand) Init(args []string) error {
	if len(args) > 0 {
		err := cmd.CheckEmpty(args[1:])
		if err != nil {
			return err
		}
		c.keys = strings.Split(args[0], ".")
	}
	return nil
}

// recurseMapOnKeys returns the value of a map keyed recursively by the
// strings given in "keys".  Thus, recurseMapOnKeys({a,b}, {a:{b:{c:d}}})
// would return {c:d}.
func recurseMapOnKeys(keys []string, params map[string]interface{}) (interface{}, bool) {
	key, rest := keys[0], keys[1:]
	answer, ok := params[key]

	// If we're out of keys, we have our answer.
	if len(rest) == 0 {
		return answer, ok
	}

	// If we're not out of keys, but we tried a key that wasn't in the
	// map, there's no answer.
	if !ok {
		return nil, false
	}

	switch typed := answer.(type) {
	// If our value is a map[s]i{}, we can keep recursing.
	case map[string]interface{}:
		return recurseMapOnKeys(keys[1:], typed)
		// If it's a map[i{}]i{}, we need to check whether it's a map[s]i{}.
	case map[interface{}]interface{}:
		m := make(map[string]interface{})
		for k, v := range typed {
			if tK, ok := k.(string); ok {
				m[tK] = v
			} else {
				// If it's not, we don't have something we
				// can work with.
				return nil, false
			}
		}
		// If it is, recurse into it.
		return recurseMapOnKeys(keys[1:], m)

		// Otherwise, we're trying to recurse into something we don't know
		// how to deal with, so our answer is that we don't have an answer.
	default:
		return nil, false
	}
}

// Run recurses into the params map for the Action, given the list of keys
// into the map, and returns either the keyed value, or nothing.
// In the case of an empty keys list, the entire params map will be returned.
func (c *ActionGetCommand) Run(ctx *cmd.Context) error {
	params, err := c.ctx.ActionParams()
	if err != nil {
		return err
	}

	var answer interface{}

	if len(c.keys) == 0 {
		// If no parameters were returned we still want to print an
		// empty object, not nil.
		if params == nil {
			answer = make(map[string]interface{})
		} else {
			answer = params
		}
	} else {
		answer, _ = recurseMapOnKeys(c.keys, params)
	}

	return c.out.Write(ctx, answer)
}
