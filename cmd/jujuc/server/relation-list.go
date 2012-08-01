package server

import (
	"fmt"
	"launchpad.net/gnuflag"
	"launchpad.net/juju-core/cmd"
)

// RelationListCommand implements the relation-list command.
type RelationListCommand struct {
	*HookContext
	RelationId int
	out        cmd.Output
}

func NewRelationListCommand(ctx *HookContext) (cmd.Command, error) {
	return &RelationListCommand{HookContext: ctx}, nil
}

func (c *RelationListCommand) Info() *cmd.Info {
	args := "<id>"
	_, id := c.relationIdentifiers()
	if id != "" {
		args = fmt.Sprintf("[<id (= %s)>]", id)
	}
	return &cmd.Info{
		"relation-list", args, "list relation units", "",
	}
}

func (c *RelationListCommand) Init(f *gnuflag.FlagSet, args []string) (err error) {
	c.out.AddFlags(f, "smart", cmd.DefaultFormatters)
	if err := f.Parse(true, args); err != nil {
		return err
	}
	args = f.Args()
	v := c.relationIdValue(&c.RelationId)
	if len(args) > 0 {
		if err := v.Set(args[0]); err != nil {
			return err
		}
		args = args[1:]
	}
	if c.RelationId == -1 {
		return fmt.Errorf("no relation specified")
	}
	return cmd.CheckEmpty(args)
}

func (c *RelationListCommand) Run(ctx *cmd.Context) error {
	return c.out.Write(ctx, c.Relations[c.RelationId].Units())
}
