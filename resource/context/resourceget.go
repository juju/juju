// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package context

import (
	"fmt"

	"github.com/juju/cmd"
	"github.com/juju/errors"
)

const ResourceGetCmdName = "resource-get"

func NewResourceGetCmd(c HookContext) (*ResourceGetCmd, error) {
	return &ResourceGetCmd{
		hookContext: c,
	}, nil
}

type ResourceGetCmd struct {
	cmd.CommandBase

	hookContext  HookContext
	resourceName string
}

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

func (c ResourceGetCmd) Init(args []string) error {
	if len(args) < 1 {
		return errors.Errorf("missing required resource name")
	}

	c.resourceName = args[0]
	return nil
}

func (c ResourceGetCmd) Run(ctx *cmd.Context) error {
	filePath, err := c.hookContext.GetResource(c.resourceName)
	if err != nil {
		return errors.Annotate(err, "could not get resource")
	}

	if _, err := fmt.Fprintf(ctx.Stdout, filePath); err != nil {
		return errors.Annotate(err, "could not write resource path to stdout")
	}
	return nil
}
