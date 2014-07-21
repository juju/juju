// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package jujuc

import (
	"fmt"
	"strings"

	"github.com/juju/cmd"
	"launchpad.net/gnuflag"

	"github.com/juju/juju/state/api/params"
)

// ActionGetCommand implements the relation-get command.
type ActionGetCommand struct {
	cmd.CommandBase
	ctx    Context
	Params map[string]interface{}
	Keys   []string
	out    cmd.Output
}

func NewActionGetCommand(ctx Context) cmd.Command {
	return &ActionGetCommand{ctx: ctx}
}

func (c *ActionGetCommand) Info() *cmd.Info {
	args := "<key>[.<key>.<key>...]"
	doc := `
action-get prints the values of the indicated parameter in the passed params
map.  If the value is a map, the values will be printed recursively as YAML.
`
	return &cmd.Info{
		Name:    "action-get",
		Args:    args,
		Purpose: "get action params",
		Doc:     doc,
	}
}

func (c *ActionGetCommand) SetFlags(f *gnuflag.FlagSet) {
	c.out.AddFlags(f, "smart", cmd.DefaultFormatters)
	f.Var(newRelationIdValue(c.ctx, &c.RelationId), "k", "specify a relation by id")
}

func (c *ActionGetCommand) Init(args []string) error {
	c.Keys = make([]string)
	c.Params = c.ctx.ActionParams()
	if len(args) > 0 {
		c.Keys = strings.Split(args[0], ".")
		args = args[1:]
		err := cmd.CheckEmpty(args)
		if err != nil {
			return err
		}

		k, ok := recurseMapOnKeys(c.Keys, c.Params)
		if !ok {
			return fmt.Errorf("key %q not found in params", k)
		}
	}
	return nil
}

func recurseMapOnKeys(keys []string, params map[string]interface{}) (interface{}, bool) {
	key, rest := keys[0], keys[1:]
	ans, ok := params[key]

	if len(rest) == 0 {
		if !ok {
			return key, false
		}
		return ans, true
	}

	switch typed := ans.(type) {
	case map[string]interface{}:
		return getActionParamAt(rest, typed)
	default:
		return rest[0], false
	}
}

func (c *ActionGetCommand) Run(ctx *cmd.Context) error {
	r, found := c.ctx.Relation(c.RelationId)
	if !found {
		return fmt.Errorf("unknown relation id")
	}
	var settings params.RelationSettings
	if c.UnitName == c.ctx.UnitName() {
		node, err := r.Settings()
		if err != nil {
			return err
		}
		settings = node.Map()
	} else {
		var err error
		settings, err = r.ReadSettings(c.UnitName)
		if err != nil {
			return err
		}
	}
	if c.Key == "" {
		return c.out.Write(ctx, settings)
	}
	if value, ok := settings[c.Key]; ok {
		return c.out.Write(ctx, value)
	}
	return c.out.Write(ctx, nil)
}
