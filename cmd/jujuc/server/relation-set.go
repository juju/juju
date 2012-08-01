package server

import (
	"fmt"
	"launchpad.net/gnuflag"
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
		"relation-set", "<key>=<value> [...]", "set relation settings", "",
	}
}

func (c *RelationSetCommand) Init(f *gnuflag.FlagSet, args []string) error {
	f.Var(c.relationIdValue(&c.RelationId), "r", "specify a relation by id")
	if err := f.Parse(true, args); err != nil {
		return err
	}
	if c.RelationId == -1 {
		return fmt.Errorf("no relation specified")
	}
	args = f.Args()
	if len(args) == 0 {
		return fmt.Errorf("no settings specified")
	}
	for _, kv := range args {
		parts := strings.SplitN(kv, "=", 2)
		if parts[0] == "" {
			return fmt.Errorf("cannot parse %q: no key specified", kv)
		}
		c.Settings[parts[0]] = parts[1]
	}
	return nil
}

func (c *RelationSetCommand) Run(ctx *cmd.Context) (err error) {
	node, err := c.Relations[c.RelationId].Settings()
	for k, v := range c.Settings {
		if s := v.(string); s != "" {
			node.Set(k, s)
		} else {
			node.Delete(k)
		}
	}
	return nil
}
