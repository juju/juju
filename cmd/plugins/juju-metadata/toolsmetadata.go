// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"fmt"

	"launchpad.net/gnuflag"
	"launchpad.net/loggo"

	"launchpad.net/juju-core/cmd"
	"launchpad.net/juju-core/environs/config"
	"launchpad.net/juju-core/environs/filestorage"
	"launchpad.net/juju-core/environs/storage"
	"launchpad.net/juju-core/environs/sync"
	"launchpad.net/juju-core/environs/tools"
	"launchpad.net/juju-core/provider/ec2/httpstorage"
	coretools "launchpad.net/juju-core/tools"
	"launchpad.net/juju-core/utils"
	"launchpad.net/juju-core/version"
)

var DefaultToolsLocation = sync.DefaultToolsLocation

// ToolsMetadataCommand is used to generate simplestreams metadata for
// juju tools.
type ToolsMetadataCommand struct {
	cmd.EnvCommandBase
	fetch       bool
	metadataDir string
	public      bool

	// noPublic is used in testing to disable the use of public storage as a backup.
	noPublic bool
}

func (c *ToolsMetadataCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "generate-tools",
		Purpose: "generate simplestreams tools metadata",
	}
}

func (c *ToolsMetadataCommand) SetFlags(f *gnuflag.FlagSet) {
	c.EnvCommandBase.SetFlags(f)
	f.StringVar(&c.metadataDir, "d", "", "local directory in which to store metadata")
	f.BoolVar(&c.public, "public", false, "tools are for a public cloud, so generate mirrors information")
}

func (c *ToolsMetadataCommand) Run(context *cmd.Context) error {
	loggo.RegisterWriter("toolsmetadata", cmd.NewCommandLogWriter("juju.environs.tools", context.Stdout, context.Stderr), loggo.INFO)
	defer loggo.RemoveWriter("toolsmetadata")
	if c.metadataDir == "" {
		c.metadataDir = config.JujuHome()
	}
	c.metadataDir = utils.NormalizePath(c.metadataDir)

	sourceStorage, err := filestorage.NewFileStorageReader(c.metadataDir)
	if err != nil {
		return err
	}
	fmt.Fprintln(context.Stdout, "Finding tools...")
	const minorVersion = -1
	toolsList, err := tools.ReadList(sourceStorage, version.Current.Major, minorVersion)
	if err == tools.ErrNoTools && !c.noPublic {
		sourceStorage = httpstorage.NewHTTPStorageReader(sync.DefaultToolsLocation)
		toolsList, err = tools.ReadList(sourceStorage, version.Current.Major, minorVersion)
	}
	if err != nil {
		return err
	}

	targetStorage, err := filestorage.NewFileStorageWriter(c.metadataDir, filestorage.UseDefaultTmpDir)
	if err != nil {
		return err
	}
	writeMirrors := tools.DoNotWriteMirrors
	if c.public {
		writeMirrors = tools.DoWriteMirrors
	}
	return mergeAndWriteMetadata(targetStorage, toolsList, writeMirrors)
}

// This is essentially the same as tools.MergeAndWriteMetadata, but also
// resolves metadata for existing tools by fetching them and computing
// size/sha256 locally.
func mergeAndWriteMetadata(stor storage.Storage, toolsList coretools.List, writeMirrors tools.WriteMirrors) error {
	existing, err := tools.ReadMetadata(stor)
	if err != nil {
		return err
	}
	metadata := tools.MetadataFromTools(toolsList)
	if metadata, err = tools.MergeMetadata(metadata, existing); err != nil {
		return err
	}
	if err = tools.ResolveMetadata(stor, metadata); err != nil {
		return err
	}
	return tools.WriteMetadata(stor, metadata, writeMirrors)
}
