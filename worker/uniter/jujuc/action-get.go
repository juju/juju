// Copyright 2012, 2013 Canonical Ltd.
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
	params := c.ctx.ActionParams()

	if len(args) > 0 {
		keys := strings.Split(args[0], ".")
		err := cmd.CheckEmpty(args[1:])
		if err != nil {
			return err
		}

		c.response, _ = recurseMapOnKeys(keys, params)
	} else if params != nil {
		c.response = params
	}

	return nil
}

func recurseMapOnKeys(keys []string, params map[string]interface{}) (interface{}, bool) {
	key, rest := keys[0], keys[1:]
	ans, ok := params[key]

	if len(rest) == 0 {
		return ans, ok
	}

	if !ok {
		return interface{}(nil), ok
	}

	switch typed := ans.(type) {
	case map[string]interface{}:
		return recurseMapOnKeys(rest, typed)
	default:
		return rest[0], false
	}
}

func (c *ActionGetCommand) Run(ctx *cmd.Context) error {
	return c.out.Write(ctx, c.response)
}
