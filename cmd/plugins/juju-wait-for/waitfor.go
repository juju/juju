// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"os"

	"github.com/juju/cmd"
	"github.com/juju/featureflag"
	"github.com/juju/loggo"

	"github.com/juju/juju/juju"
	"github.com/juju/juju/juju/osenv"
	_ "github.com/juju/juju/provider/all"
)

var logger = loggo.GetLogger("juju.plugins.waitfor")

var waitForDoc = `
Juju wait-for attempts to wait for a given entity to reach a goal state.
`

// Main registers subcommands for the juju-metadata executable, and hands over control
// to the cmd package. This function is not redundant with main, because it
// provides an entry point for testing with arbitrary command line arguments.
func Main(args []string) {
	ctx, err := cmd.DefaultContext()
	if err != nil {
		cmd.WriteError(os.Stderr, err)
		os.Exit(2)
	}
	if err := juju.InitJujuXDGDataHome(); err != nil {
		cmd.WriteError(ctx.Stderr, err)
		os.Exit(2)
	}
	os.Exit(cmd.Main(NewSuperCommand(), ctx, args[1:]))
}

// NewSuperCommand creates the metadata plugin supercommand and registers the
// subcommands that it supports.
func NewSuperCommand() cmd.Command {
	waitFor := cmd.NewSuperCommand(cmd.SuperCommandParams{
		Name:        "wait-for",
		UsagePrefix: "juju",
		Doc:         waitForDoc,
		Purpose:     "tools for generating and validating image and tools metadata",
		Log:         &cmd.Log{}})

	waitFor.Register(newApplicationCommand())
	waitFor.Register(newModelCommand())
	waitFor.Register(newUnitCommand())
	return waitFor
}

func init() {
	featureflag.SetFlagsFromEnvironment(osenv.JujuFeatureFlagEnvKey, osenv.JujuFeatures)
}

func main() {
	Main(os.Args)
}
