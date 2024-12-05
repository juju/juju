// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the LGPLv3, see LICENSE file for details.

package cmd

import (
	"github.com/juju/gnuflag"
)

// versionCommand is a cmd.Command that prints the current version.
type versionCommand struct {
	CommandBase
	out           Output
	version       string
	versionDetail interface{}

	showAll bool
}

func newVersionCommand(version string, versionDetail interface{}) *versionCommand {
	return &versionCommand{
		version:       version,
		versionDetail: versionDetail,
	}
}

func (v *versionCommand) Info() *Info {
	return &Info{
		Name:    "version",
		Purpose: "Print the current version.",
	}
}

func (v *versionCommand) SetFlags(f *gnuflag.FlagSet) {
	formatters := make(map[string]Formatter, len(DefaultFormatters))
	for k, v := range DefaultFormatters {
		formatters[k] = v.Formatter
	}
	v.out.AddFlags(f, "smart", formatters)
	f.BoolVar(&v.showAll, "all", false, "Prints all version information")
}

func (v *versionCommand) Run(ctxt *Context) error {
	if v.showAll {
		return v.out.Write(ctxt, v.versionDetail)
	}
	return v.out.Write(ctxt, v.version)
}
