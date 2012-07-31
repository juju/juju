package server

import (
	"fmt"
	"launchpad.net/gnuflag"
	"launchpad.net/juju-core/cmd"
	"strconv"
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
	_, defaultId := c.relationIdentifiers()
	relationId := ""
	f.StringVar(&relationId, "r", defaultId, "relation id")
	if err := f.Parse(true, args); err != nil {
		return err
	}
	if relationId == "" {
		if c.HookContext.RelationId == -1 {
			return fmt.Errorf("no relation specified")
		}
		c.RelationId = c.HookContext.RelationId
	} else {
		trim := relationId
		if idx := strings.LastIndex(trim, ":"); idx != -1 {
			trim = trim[idx+1:]
		}
		id, err := strconv.Atoi(trim)
		if err != nil {
			return fmt.Errorf("invalid relation id %q", relationId)
		}
		if _, found := c.Relations[id]; !found {
			return fmt.Errorf("unknown relation id %q", relationId)
		}
		c.RelationId = id
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
