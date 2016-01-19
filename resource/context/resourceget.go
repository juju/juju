// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package context

import (
	"fmt"

	"github.com/juju/cmd"
	"github.com/juju/errors"
)

// ResourceGetCmdName is the name of the resource-get command.
const ResourceGetCmdName = "resource-get"

// NewResourceGetCmd creates a new ResourceGetCmd for the given hook context.
func NewResourceGetCmd(c HookContext) (*ResourceGetCmd, error) {
	return &ResourceGetCmd{
		hookContext: c,
	}, nil
}

// ResourceGetCmd provides the functionality of the resource-get command.
type ResourceGetCmd struct {
	cmd.CommandBase

	hookContext  HookContext
	resourceName string
}

// Info implements cmd.Command.
func (c ResourceGetCmd) Info() *cmd.Info {
	return &cmd.Info{
		Name: ResourceGetCmdName,
		Args: "<resource name>",
		// TODO(katco): Implement
		Purpose: "",
		Doc: `
`,
	}
}

// Init implements cmd.Command.
func (c *ResourceGetCmd) Init(args []string) error {
	if len(args) < 1 {
		return errors.Errorf("missing required resource name")
	} else if err := cmd.CheckEmpty(args[1:]); err != nil {
		return errors.Trace(err)
	}
	c.resourceName = args[0]
	return nil
}

// Run implements cmd.Command.
func (c ResourceGetCmd) Run(ctx *cmd.Context) error {
	filePath, err := c.hookContext.DownloadResource(c.resourceName)
	if err != nil {
		return errors.Annotate(err, "could not download resource")
	}

	if _, err := fmt.Fprintf(ctx.Stdout, filePath); err != nil {
		return errors.Annotate(err, "could not write resource path to stdout")
	}
	return nil
}
