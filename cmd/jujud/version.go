package main

import (
	"launchpad.net/gnuflag"
	"launchpad.net/juju-core/cmd"
	"launchpad.net/juju-core/version"
)

// VersionCommand is a cmd.Command that prints the current version.
type VersionCommand struct {
	out cmd.Output
}

func (v *VersionCommand) Info() *cmd.Info {
	return &cmd.Info{"version", "", "print the current version", ""}
}

func (v *VersionCommand) Init(f *gnuflag.FlagSet, args []string) error {
	v.out.AddFlags(f, "smart", cmd.DefaultFormatters)
	if err := f.Parse(true, args); err != nil {
		return err
	}
	return cmd.CheckEmpty(f.Args())
}

func (v *VersionCommand) Run(ctxt *cmd.Context) error {
	return v.out.Write(ctxt, version.Current.String())
}
