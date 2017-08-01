// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resource

import (
	"github.com/juju/cmd"
	"github.com/juju/gnuflag"

	"github.com/juju/juju/cmd/modelcmd"
)

const (
	Deprecated      = "DEPRECATED: "
	DeprecatedSince = "This command is DEPRECATED from Juju 2.3.x\n"
)

// ListCharmResourcesCommand implements the "juju charm resources" command.
type ListCharmResourcesCommand struct {
	baseCharmResourcesCommand
}

// NewListCharmResourcesCommand returns a new command that lists resources defined
// by a charm.
func NewListCharmResourcesCommand(resourceLister ResourceLister) modelcmd.ModelCommand {
	var c ListCharmResourcesCommand
	c.setResourceLister(resourceLister)
	return modelcmd.Wrap(&c)
}

// Info implements cmd.Command.
func (c *ListCharmResourcesCommand) Info() *cmd.Info {
	i := c.baseInfo()
	i.Name = "resources"
	i.Aliases = []string{"list-resources"}
	i.Doc = DeprecatedSince + i.Doc
	i.Purpose = Deprecated + i.Purpose
	return i
}

// SetFlags implements cmd.Command.
func (c *ListCharmResourcesCommand) SetFlags(f *gnuflag.FlagSet) {
	c.setBaseFlags(f)
}

// Init implements cmd.Command.
func (c *ListCharmResourcesCommand) Init(args []string) error {
	return c.baseInit(args)
}

// Run implements cmd.Command.
func (c *ListCharmResourcesCommand) Run(ctx *cmd.Context) error {
	ctx.Warningf(DeprecatedSince)
	return c.baseRun(ctx)
}
