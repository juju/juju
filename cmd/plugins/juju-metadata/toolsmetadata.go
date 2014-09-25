// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"fmt"

	"github.com/juju/cmd"
	"github.com/juju/loggo"
	"github.com/juju/utils"
	"launchpad.net/gnuflag"

	"github.com/juju/juju/cmd/envcmd"
	"github.com/juju/juju/environs/filestorage"
	"github.com/juju/juju/environs/simplestreams"
	"github.com/juju/juju/environs/storage"
	envtools "github.com/juju/juju/environs/tools"
	"github.com/juju/juju/juju/osenv"
	coretools "github.com/juju/juju/tools"
	"github.com/juju/juju/version"
)

// ToolsMetadataCommand is used to generate simplestreams metadata for juju tools.
type ToolsMetadataCommand struct {
	envcmd.EnvCommandBase
	fetch       bool
	metadataDir string
	stream      string
	public      bool
}

func (c *ToolsMetadataCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "generate-tools",
		Purpose: "generate simplestreams tools metadata",
	}
}

func (c *ToolsMetadataCommand) SetFlags(f *gnuflag.FlagSet) {
	f.StringVar(&c.metadataDir, "d", "", "local directory in which to store metadata")
	f.StringVar(&c.stream, "stream", envtools.ReleasedStream, "simplestreams stream for which to generate the metadata")
	f.BoolVar(&c.public, "public", false, "tools are for a public cloud, so generate mirrors information")
}

func (c *ToolsMetadataCommand) Run(context *cmd.Context) error {
	loggo.RegisterWriter("toolsmetadata", cmd.NewCommandLogWriter("juju.environs.tools", context.Stdout, context.Stderr), loggo.INFO)
	defer loggo.RemoveWriter("toolsmetadata")
	if c.metadataDir == "" {
		c.metadataDir = osenv.JujuHome()
	} else {
		c.metadataDir = context.AbsPath(c.metadataDir)
	}

	sourceStorage, err := filestorage.NewFileStorageReader(c.metadataDir)
	if err != nil {
		return err
	}
	fmt.Fprintf(context.Stdout, "Finding tools in %s\n", c.metadataDir)
	const minorVersion = -1
	toolsList, err := envtools.ReadList(sourceStorage, version.Current.Major, minorVersion)
	if err == envtools.ErrNoTools {
		var source string
		source, err = envtools.ToolsURL(envtools.DefaultBaseURL)
		if err != nil {
			return err
		}
		sourceDataSource := simplestreams.NewURLDataSource("local source", source, utils.VerifySSLHostnames)
		toolsList, err = envtools.FindToolsForCloud(
			[]simplestreams.DataSource{sourceDataSource}, simplestreams.CloudSpec{}, c.stream,
			version.Current.Major, minorVersion, coretools.Filter{})
	}
	if err != nil {
		return err
	}

	targetStorage, err := filestorage.NewFileStorageWriter(c.metadataDir)
	if err != nil {
		return err
	}
	writeMirrors := envtools.DoNotWriteMirrors
	if c.public {
		writeMirrors = envtools.WriteMirrors
	}
	return mergeAndWriteMetadata(targetStorage, c.stream, toolsList, writeMirrors)
}

// This is essentially the same as tools.MergeAndWriteMetadata, but also
// resolves metadata for existing tools by fetching them and computing
// size/sha256 locally.
func mergeAndWriteMetadata(stor storage.Storage, stream string, toolsList coretools.List, writeMirrors envtools.ShouldWriteMirrors) error {
	existing, err := envtools.ReadMetadata(stor, stream)
	if err != nil {
		return err
	}
	metadata := envtools.MetadataFromTools(toolsList)
	if metadata, err = envtools.MergeMetadata(metadata, existing); err != nil {
		return err
	}
	if err = envtools.ResolveMetadata(stor, metadata); err != nil {
		return err
	}
	return envtools.WriteMetadata(stor, stream, metadata, writeMirrors)
}
