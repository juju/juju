package jujuc

import (
	"fmt"
	"launchpad.net/gnuflag"
	"launchpad.net/juju-core/cmd"
	"sort"
)

// RelationIdsCommand implements the relation-ids command.
type RelationIdsCommand struct {
	ctx  Context
	Name string
	out  cmd.Output
}

func NewRelationIdsCommand(ctx Context) cmd.Command {
	return &RelationIdsCommand{ctx: ctx}
}

func (c *RelationIdsCommand) Info() *cmd.Info {
	args := "<name>"
	doc := ""
	if c.ctx.HasHookRelation() {
		args = "[<name>]"
		doc = fmt.Sprintf("Current default relation name is %q.", c.ctx.HookRelation().Name())
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
	if c.ctx.HasHookRelation() {
		c.Name = c.ctx.HookRelation().Name()
	}
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
	for _, id := range c.ctx.RelationIds() {
		r := c.ctx.Relation(id)
		if r.Name() == c.Name {
			result = append(result, r.FakeId())
		}
	}
	sort.Strings(result)
	return c.out.Write(ctx, result)
}
