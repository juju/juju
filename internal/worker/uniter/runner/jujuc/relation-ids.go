// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package jujuc

import (
	"fmt"
	"sort"

	"github.com/juju/cmd/v4"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"

	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/core/life"
)

// RelationIdsCommand implements the relation-ids command.
type RelationIdsCommand struct {
	cmd.CommandBase
	ctx  Context
	Name string
	out  cmd.Output
}

func NewRelationIdsCommand(ctx Context) (cmd.Command, error) {
	name := ""
	if r, err := ctx.HookRelation(); err == nil {
		name = r.Name()
	} else if !errors.Is(err, errors.NotFound) {
		return nil, errors.Trace(err)
	}

	return &RelationIdsCommand{ctx: ctx, Name: name}, nil
}

func (c *RelationIdsCommand) Info() *cmd.Info {
	args := "<name>"
	doc := ""
	if r, err := c.ctx.HookRelation(); err == nil {
		// There's not much we can do about this error here.
		args = "[<name>]"
		doc = fmt.Sprintf("Current default endpoint name is %q.", r.Name())
	} else if !errors.Is(err, errors.NotFound) {
		logger.Errorf("Could not retrieve hook relation: %v", err)
	}
	doc += `
relation-ids outputs a list of the related applications with a relation name.
Accepts a single argument (relation-name) which, in a relation hook, defaults
to the name of the current relation. The output is useful as input to the
relation-list, relation-get, and relation-set commands to read or write other
relation values.

Only relation ids for relations which are not broken are included.
`
	return jujucmd.Info(&cmd.Info{
		Name:    "relation-ids",
		Args:    args,
		Purpose: "List all relation IDs for the given endpoint.",
		Doc:     doc,
		Examples: `
    relation-ids database
`,
	})
}

func (c *RelationIdsCommand) SetFlags(f *gnuflag.FlagSet) {
	c.out.AddFlags(f, "smart", cmd.DefaultFormatters.Formatters())
}

func (c *RelationIdsCommand) Init(args []string) error {
	if len(args) > 0 {
		c.Name = args[0]
		args = args[1:]
	} else if c.Name == "" {
		return fmt.Errorf("no endpoint name specified")
	}
	return cmd.CheckEmpty(args)
}

func (c *RelationIdsCommand) Run(ctx *cmd.Context) error {
	result := []string{}
	ids, err := c.ctx.RelationIds()
	if err != nil && !errors.Is(err, errors.NotFound) {
		return errors.Trace(err)
	}
	for _, id := range ids {
		r, err := c.ctx.Relation(id)
		if err == nil && r.Name() == c.Name && r.Life() != life.Dead {
			result = append(result, r.FakeId())
		} else if err != nil && !errors.Is(err, errors.NotFound) {
			return errors.Trace(err)
		}
	}
	sort.Strings(result)
	return c.out.Write(ctx, result)
}
