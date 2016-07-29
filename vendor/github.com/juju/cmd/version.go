// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the LGPLv3, see LICENSE file for details.

package cmd

import (
	"launchpad.net/gnuflag"
)

// versionCommand is a cmd.Command that prints the current version.
type versionCommand struct {
	CommandBase
	out     Output
	version string
}

func newVersionCommand(version string) *versionCommand {
	return &versionCommand{
		version: version,
	}
}

func (v *versionCommand) Info() *Info {
	return &Info{
		Name:    "version",
		Purpose: "print the current version",
	}
}

func (v *versionCommand) SetFlags(f *gnuflag.FlagSet) {
	v.out.AddFlags(f, "smart", DefaultFormatters)
}

func (v *versionCommand) Run(ctxt *Context) error {
	return v.out.Write(ctxt, v.version)
}
