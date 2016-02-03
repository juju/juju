// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"fmt"
	"os"

	"github.com/juju/cmd"
	"github.com/juju/loggo"
	"github.com/juju/utils/featureflag"

	"github.com/juju/juju/feature"
	"github.com/juju/juju/juju"
	"github.com/juju/juju/juju/osenv"
	_ "github.com/juju/juju/provider/all"
)

var logger = loggo.GetLogger("juju.plugins.metadata")

var metadataDoc = `
Juju metadata is used to find the correct image and tools when bootstrapping a
Juju environment.
`

// Main registers subcommands for the juju-metadata executable, and hands over control
// to the cmd package. This function is not redundant with main, because it
// provides an entry point for testing with arbitrary command line arguments.
func Main(args []string) {
	ctx, err := cmd.DefaultContext()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(2)
	}
	if err := juju.InitJujuHome(); err != nil {
		fmt.Fprintf(os.Stderr, "error: %s\n", err)
		os.Exit(2)
	}
	os.Exit(cmd.Main(NewSuperCommand(), ctx, args[1:]))
}

// NewSuperCommand creates the metadata plugin supercommand and registers the
// subcommands that it supports.
func NewSuperCommand() cmd.Command {
	metadatacmd := cmd.NewSuperCommand(cmd.SuperCommandParams{
		Name:        "metadata",
		UsagePrefix: "juju",
		Doc:         metadataDoc,
		Purpose:     "tools for generating and validating image and tools metadata",
		Log:         &cmd.Log{}})

	metadatacmd.Register(newValidateImageMetadataCommand())
	metadatacmd.Register(newImageMetadataCommand())
	metadatacmd.Register(newToolsMetadataCommand())
	metadatacmd.Register(newValidateToolsMetadataCommand())
	metadatacmd.Register(newSignMetadataCommand())
	if featureflag.Enabled(feature.ImageMetadata) {
		metadatacmd.Register(newListImagesCommand())
		metadatacmd.Register(newAddImageMetadataCommand())
		metadatacmd.Register(newDeleteImageMetadataCommand())
	}
	return metadatacmd
}

func init() {
	featureflag.SetFlagsFromEnvironment(osenv.JujuFeatureFlagEnvKey)
}

func main() {
	Main(os.Args)
}
