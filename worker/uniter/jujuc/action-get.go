// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package jujuc

import (
	"strings"

	"github.com/juju/cmd"
	"launchpad.net/gnuflag"
)

// ActionGetCommand implements the relation-get command.
type ActionGetCommand struct {
	cmd.CommandBase
	ctx      Context
	keys     []string
	response interface{}
	out      cmd.Output
}

func NewActionGetCommand(ctx Context) cmd.Command {
	return &ActionGetCommand{ctx: ctx}
}

func (c *ActionGetCommand) Info() *cmd.Info {
	doc := `
action-get will print the value of the parameter at the given key, serialized
as YAML.  If multiple keys are passed, action-get will recurse into the param
map as needed.
`
	return &cmd.Info{
		Name:    "action-get",
		Args:    "[<key>[.<key>.<key>...]]",
		Purpose: "get action parameters",
		Doc:     doc,
	}
}

func (c *ActionGetCommand) SetFlags(f *gnuflag.FlagSet) {
	c.out.AddFlags(f, "smart", cmd.DefaultFormatters)
}

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

func recurseMapOnKeys(keys []string, params map[string]interface{}) (interface{}, bool) {
	key, rest := keys[0], keys[1:]
	answer, ok := params[key]

	if len(rest) == 0 {
		return answer, ok
	} else if ok {
		switch typed := answer.(type) {
		case map[string]interface{}:
			return recurseMapOnKeys(keys[1:], typed)
		case map[interface{}]interface{}:
			m := make(map[string]interface{})
			for k, v := range typed {
				if tK, ok := k.(string); ok {
					m[tK] = v
				} else {
					return nil, false
				}
			}
			return recurseMapOnKeys(keys[1:], m)
		default:
			return nil, false
		}
	} else {
		return nil, false
	}

	return nil, false
}

func (c *ActionGetCommand) Run(ctx *cmd.Context) error {
	params := c.ctx.ActionParams()

	var answer interface{}

	if len(c.keys) == 0 {
		answer = params
	} else {
		answer, _ = recurseMapOnKeys(c.keys, params)
	}

	return c.out.Write(ctx, answer)
}
