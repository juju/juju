package jujuc

import (
	"fmt"
	"launchpad.net/gnuflag"
	"launchpad.net/juju-core/cmd"
	"sort"
)

// RelationIdsCommand implements the relation-ids command.
type RelationIdsCommand struct {
	*HookContext
	Name string
	out  cmd.Output
}

func NewRelationIdsCommand(ctx *HookContext) (cmd.Command, error) {
	return &RelationIdsCommand{HookContext: ctx}, nil
}

func (c *RelationIdsCommand) Info() *cmd.Info {
	args := "<name>"
	doc := ""
	if name := c.envRelation(); name != "" {
		args = "[<name>]"
		doc = fmt.Sprintf("Current default relation name is %q.", name)
	}
	return &cmd.Info{
		"relation-ids", args, "list all relation ids with the given relation name", doc,
	}
}

func (c *RelationIdsCommand) Init(f *gnuflag.FlagSet, args []string) error {
	c.out.AddFlags(f, "smart", cmd.DefaultFormatters)
	if err := f.Parse(true, args); err != nil {
		return err
	}
	args = f.Args()
	c.Name = c.envRelation()
	if len(args) > 0 {
		c.Name = args[0]
		args = args[1:]
	} else if c.Name == "" {
		return fmt.Errorf("no relation name specified")
	}
	return cmd.CheckEmpty(args)
}

func (c *RelationIdsCommand) Run(ctx *cmd.Context) error {
	result := []string{}
	for id, rctx := range c.Relations {
		if rctx.ru.Endpoint().RelationName == c.Name {
			result = append(result, fmt.Sprintf("%s:%d", c.Name, id))
		}
	}
	sort.Strings(result)
	return c.out.Write(ctx, result)
}
