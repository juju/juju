// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"fmt"
	"launchpad.net/juju-core/cmd"
	_ "launchpad.net/juju-core/environs/all"
	"launchpad.net/juju-core/juju"
	"os"
)

var metadataDoc = `
juju metadata provides tools for generating and validating image and tools metadata.

The metadata is used to find the correct image and tools when bootstrapping a Juju
environment.
`

// Main registers subcommands for the juju-metadata executable, and hands over control
// to the cmd package. This function is not redundant with main, because it
// provides an entry point for testing with arbitrary command line arguments.
func Main(args []string) {
	if err := juju.InitJujuHome(); err != nil {
		fmt.Fprintf(os.Stderr, "error: %s\n", err)
		os.Exit(2)
	}
	jujucmd := cmd.NewSuperCommand(cmd.SuperCommandParams{
		Name: "metadata",
		Doc:  metadataDoc,
		Log:  &cmd.Log{},
	})

	jujucmd.Register(&ValidateImageMetadataCommand{})
	jujucmd.Register(&ImageMetadataCommand{})

	os.Exit(cmd.Main(jujucmd, cmd.DefaultContext(), args[1:]))
}

func main() {
	Main(os.Args)
}
