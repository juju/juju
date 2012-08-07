package server

import (
	"fmt"
	"launchpad.net/gnuflag"
	"launchpad.net/juju-core/cmd"
)

// RelationGetCommand implements the relation-get command.
type RelationGetCommand struct {
	*HookContext
	RelationId int
	Key        string
	UnitName   string
	out        cmd.Output
}

func NewRelationGetCommand(ctx *HookContext) (cmd.Command, error) {
	return &RelationGetCommand{HookContext: ctx}, nil
}

func (c *RelationGetCommand) Info() *cmd.Info {
	args := "<key> <unit id>"
	doc := `
relation-get prints the value of a unit's relation setting, specified by key.
If no key is given, or if the key is "-", all keys and values will be printed.
`
	if c.RemoteUnitName != "" {
		args = "[<key> [<unit id>]]"
		doc += fmt.Sprintf("Current default unit id is %q.", c.RemoteUnitName)
	}
	return &cmd.Info{
		"relation-get", args, "get relation settings", doc,
	}
}

func (c *RelationGetCommand) Init(f *gnuflag.FlagSet, args []string) error {
	// TODO FWER implement --format shell lp:1033511
	c.out.AddFlags(f, "smart", cmd.DefaultFormatters)
	relationId, err := c.parseRelationId(f, args)
	if err != nil {
		return err
	}
	c.RelationId = relationId
	args = f.Args()
	c.Key = ""
	if len(args) > 0 {
		if c.Key = args[0]; c.Key == "-" {
			c.Key = ""
		}
		args = args[1:]
	}
	c.UnitName = c.RemoteUnitName
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
	var settings map[string]interface{}
	if c.UnitName == c.Unit.Name() {
		node, err := c.Relations[c.RelationId].Settings()
		if err != nil {
			return err
		}
		settings = node.Map()
	} else {
		var err error
		settings, err = c.Relations[c.RelationId].ReadSettings(c.UnitName)
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
