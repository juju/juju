package jujuc

import (
	"fmt"
	"launchpad.net/gnuflag"
	"launchpad.net/juju-core/cmd"
)

// RelationGetCommand implements the relation-get command.
type RelationGetCommand struct {
	ctx        Context
	RelationId int
	Key        string
	UnitName   string
	out        cmd.Output
}

func NewRelationGetCommand(ctx Context) cmd.Command {
	return &RelationGetCommand{ctx: ctx}
}

func (c *RelationGetCommand) Info() *cmd.Info {
	args := "<key> <unit id>"
	doc := `
relation-get prints the value of a unit's relation setting, specified by key.
If no key is given, or if the key is "-", all keys and values will be printed.
`
	if name, found := c.ctx.RemoteUnitName(); found {
		args = "[<key> [<unit id>]]"
		doc += fmt.Sprintf("Current default unit id is %q.", name)
	}
	return &cmd.Info{
		"relation-get", args, "get relation settings", doc,
	}
}

func (c *RelationGetCommand) Init(f *gnuflag.FlagSet, args []string) error {
	// TODO FWER implement --format shell lp:1033511
	c.out.AddFlags(f, "smart", cmd.DefaultFormatters)
	f.Var(newRelationIdValue(c.ctx, &c.RelationId), "r", "specify a relation by id")
	if err := f.Parse(true, args); err != nil {
		return err
	}
	if c.RelationId == -1 {
		return fmt.Errorf("no relation id specified")
	}
	args = f.Args()
	c.Key = ""
	if len(args) > 0 {
		if c.Key = args[0]; c.Key == "-" {
			c.Key = ""
		}
		args = args[1:]
	}
	if name, found := c.ctx.RemoteUnitName(); found {
		c.UnitName = name
	}
	if len(args) > 0 {
		c.UnitName = args[0]
		args = args[1:]
	}
	if c.UnitName == "" {
		return fmt.Errorf("no unit id specified")
	}
	return cmd.CheckEmpty(args)
}

func (c *RelationGetCommand) Run(ctx *cmd.Context) error {
	r, found := c.ctx.Relation(c.RelationId)
	if !found {
		return fmt.Errorf("unknown relation id")
	}
	var settings map[string]interface{}
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
	var value interface{}
	if c.Key == "" {
		value = settings
	} else {
		value, _ = settings[c.Key]
	}
	return c.out.Write(ctx, value)
}
