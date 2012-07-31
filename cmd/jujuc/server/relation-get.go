package server

import (
	"fmt"
	"launchpad.net/gnuflag"
	"launchpad.net/juju-core/cmd"
	"launchpad.net/juju-core/state"
)

// RelationGetCommand implements the relation-get command.
type RelationGetCommand struct {
	*HookContext
	RelationId int
	UnitName   string
	Key        string
	out        cmd.Output
	testMode   bool
}

func NewRelationGetCommand(ctx *HookContext) (cmd.Command, error) {
	return &RelationGetCommand{HookContext: ctx}, nil
}

func (c *RelationGetCommand) Info() *cmd.Info {
	args := "<key> <unit>"
	if c.RemoteUnitName != "" {
		args = fmt.Sprintf("[<key> [<unit (= %q)]]", c.RemoteUnitName)
	}
	return &cmd.Info{
		"relation-get", args, "get relation settings", `
Specifying a key will cause a single settings value to be returned. Leaving
key empty, or setting it to "-", will cause all keys and values to be returned.
`,
	}
}

func (c *RelationGetCommand) Init(f *gnuflag.FlagSet, args []string) error {
	c.out.AddFlags(f, "yaml", relationGetFormatters)
	f.BoolVar(&c.testMode, "test", false, "returns non-zero exit code if value is false/zero/empty")
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
	}
	c.UnitName = c.RemoteUnitName
	if len(args) > 1 {
		c.UnitName = args[1]
	}
	if c.UnitName == "" {
		return fmt.Errorf("unit not specified")
	}
	return cmd.CheckEmpty(args[2:])
}

func (c *RelationGetCommand) Run(ctx *cmd.Context) (err error) {
	var settings map[string]interface{}
	if c.UnitName == c.Unit.Name() {
		var node *state.ConfigNode
		node, err = c.Relations[c.RelationId].Settings()
		settings = node.Map()
	} else {
		settings, err = c.Relations[c.RelationId].ReadSettings(c.UnitName)
	}
	if err != nil {
		return err
	}
	var value interface{}
	if c.Key == "" {
		value = settings
	} else {
		value, _ = settings[c.Key]
	}
	if c.testMode {
		return truthError(value)
	}
	return c.out.Write(ctx, value)
}

var relationGetFormatters = map[string]cmd.Formatter{}

func init() {
	for name, f := range cmd.DefaultFormatters {
		relationGetFormatters[name] = f
	}
	relationGetFormatters["shell"] = formatShell
}

func formatShell(value interface{}) ([]byte, error) {
	panic(value)
}
