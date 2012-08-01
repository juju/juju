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
	args := "<key> <unit>"
	if c.RemoteUnitName != "" {
		args = fmt.Sprintf("[<key> [<unit (= %s)>]]", c.RemoteUnitName)
	}
	return &cmd.Info{
		"relation-get", args, "get relation settings", `
Specifying a key will cause a single settings value to be written to stdout.
If the value does not exist, nothing is written. Leaving key empty, or setting
it to "-", will cause all keys and values to be written.
`,
	}
}

func (c *RelationGetCommand) Init(f *gnuflag.FlagSet, args []string) error {
	c.out.AddFlags(f, "smart", cmd.DefaultFormatters)
	f.Var(c.relationIdValue(&c.RelationId), "r", "specify a relation by id")
	if err := f.Parse(true, args); err != nil {
		return err
	}
	if c.RelationId == -1 {
		return fmt.Errorf("no relation specified")
	}
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
		return fmt.Errorf("no unit specified")
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
