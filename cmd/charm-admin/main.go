// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"os"

	"launchpad.net/juju-core/cmd"
)

func main() {
	admcmd := cmd.NewSuperCommand(cmd.SuperCommandParams{
		Name: "charm-admin",
		Log:  &cmd.Log{},
	})

	admcmd.Register(&DeleteCharmCommand{})

	os.Exit(cmd.Main(admcmd, cmd.DefaultContext(), os.Args[1:]))
}
