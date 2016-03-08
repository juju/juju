// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cmd

import (
	"fmt"

	"github.com/juju/cmd"
	"github.com/juju/errors"
)

// GetCmdName is the name of the resource-get command.
const GetCmdName = "resource-get"

// NewGetCmd creates a new GetCmd for the given hook context.
func NewGetCmd(c HookContext) (*GetCmd, error) {
	return &GetCmd{
		hookContext: c,
	}, nil
}

// GetCmd provides the functionality of the resource-get command.
type GetCmd struct {
	cmd.CommandBase

	hookContext  HookContext
	resourceName string
}

// TODO(ericsnow) Also provide an indicator of whether or not
// the resource has changed (in addition to the file path)?

// Info implements cmd.Command.
func (c GetCmd) Info() *cmd.Info {
	return &cmd.Info{
		Name:    GetCmdName,
		Args:    "<resource name>",
		Purpose: "get the path to the locally cached resource file",
		Doc: `
"resource-get" is used while a hook is running to get the local path
to the file for the identified resource. This file is an fs-local copy,
unique to the unit for which the hook is running. It is downloaded from
the controller, if necessary.

If "resource-get" for a resource has not been run before (for the unit)
then the resource is downloaded from the controller at the revision
associated with the unit's service. That file is stored in the unit's
local cache. If "resource-get" *has* been run before then each
subsequent run syncs the resource with the controller. This ensures
that the revision of the unit-local copy of the resource matches the
revision of the resource associated with the unit's service.

Either way, the path provided by "resource-get" references the
up-to-date file for the resource. Note that the resource may get
updated on the controller for the service at any time, meaning the
cached copy *may* be out of date at any time after you call
"resource-get". Consequently, the command should be run at every
point where it is critical that the resource be up to date.

The "upgrade-charm" hook is useful for keeping your charm's resources
on a unit up to date.  Run "resource-get" there for each of your
charm's resources to do so. The hook fires whenever the the file for
one of the service's resources changes on the controller (in addition
to when the charm itself changes). That means it happens in response
to "juju upgrade-charm" as well as to "juju push-resource".

Note that the "upgrade-charm" hook does not run when the unit is
started up. So be sure to run "resource-get" for your resources in the
"install" hook (or "config-changed", etc.).

Note that "resource-get" only provides an FS path to the resource file.
It does not provide any information about the resource (e.g. revision).
`,
	}
}

// Init implements cmd.Command.
func (c *GetCmd) Init(args []string) error {
	if len(args) < 1 {
		return errors.Errorf("missing required resource name")
	} else if err := cmd.CheckEmpty(args[1:]); err != nil {
		return errors.Trace(err)
	}
	c.resourceName = args[0]
	return nil
}

// Run implements cmd.Command.
func (c GetCmd) Run(ctx *cmd.Context) error {
	filePath, err := c.hookContext.Download(c.resourceName)
	if err != nil {
		return errors.Annotate(err, "could not download resource")
	}

	if _, err := fmt.Fprintf(ctx.Stdout, filePath); err != nil {
		return errors.Annotate(err, "could not write resource path to stdout")
	}
	return nil
}
