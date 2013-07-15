// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cmd

import (
	"launchpad.net/gnuflag"

	"launchpad.net/juju-core/version"
)

// VersionCommand is a cmd.Command that prints the current version.
type VersionCommand struct {
	CommandBase
	out Output
}

func (v *VersionCommand) Info() *Info {
	return &Info{
		Name:    "version",
		Purpose: "print the current version",
	}
}

func (v *VersionCommand) SetFlags(f *gnuflag.FlagSet) {
	v.out.AddFlags(f, "smart", DefaultFormatters)
}

func (v *VersionCommand) Run(ctxt *Context) error {
	return v.out.Write(ctxt, version.Current.String())
}
