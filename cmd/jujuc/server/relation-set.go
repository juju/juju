package server

import (
	"fmt"
	"launchpad.net/gnuflag"
	"launchpad.net/goyaml"
	"launchpad.net/juju-core/cmd"
	"strings"
)

// RelationSetCommand implements the relation-set command.
type RelationSetCommand struct {
	*HookContext
	RelationId int
	Settings   map[string]interface{}
}

func NewRelationSetCommand(ctx *HookContext) (cmd.Command, error) {
	return &RelationSetCommand{HookContext: ctx, Settings: map[string]interface{}{}}, nil
}

func (c *RelationSetCommand) Info() *cmd.Info {
	return &cmd.Info{
		"relation-set", "<key=value> [, ...]", "set relation settings", "",
	}
}

func (c *RelationSetCommand) Init(f *gnuflag.FlagSet, args []string) error {
	relationId, err := c.parseRelationId(f, args)
	if err != nil {
		return err
	}
	c.RelationId = relationId
	args = f.Args()
	if len(args) == 0 {
		return fmt.Errorf("no settings specified")
	}
	for _, kv := range args {
		parts := strings.SplitN(kv, "=", 2)
		var err error
		var value interface{}
		if parts[0] == "" {
			err = fmt.Errorf("no key specified")
		} else {
			err = goyaml.Unmarshal([]byte(parts[1]), &value)
		}
		if err != nil {
			return fmt.Errorf("cannot parse %q: %v", kv, err)
		}
		c.Settings[parts[0]] = value
	}
	return nil
}

func (c *RelationSetCommand) Run(ctx *cmd.Context) (err error) {
	node, err := c.Relations[c.RelationId].Settings()
	for k, v := range c.Settings {
		if v != nil {
			node.Set(k, v)
		} else {
			node.Delete(k)
		}
	}
	return nil
}
