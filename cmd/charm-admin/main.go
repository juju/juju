// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"fmt"
	"os"

	"launchpad.net/juju-core/cmd"
)

func main() {
	ctx, err := cmd.DefaultContext()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(2)
	}

	admcmd := cmd.NewSuperCommand(cmd.SuperCommandParams{
		Name: "charm-admin",
		Log:  &cmd.Log{},
	})

	admcmd.Register(&DeleteCharmCommand{})

	os.Exit(cmd.Main(admcmd, ctx, os.Args[1:]))
}
