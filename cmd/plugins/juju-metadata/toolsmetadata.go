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
	"launchpad.net/juju-core/environs/sync"
	"launchpad.net/juju-core/environs/tools"
	"launchpad.net/juju-core/provider/ec2"
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

	// noS3 is used in testing to disable the use of S3 public storage
	// as a backup.
	noS3 bool
}

func (c *ToolsMetadataCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "generate-tools",
		Purpose: "generate simplestreams tools metadata",
	}
}

func (c *ToolsMetadataCommand) SetFlags(f *gnuflag.FlagSet) {
	c.EnvCommandBase.SetFlags(f)
	f.BoolVar(&c.fetch, "fetch", true, "fetch tools and compute content size and hash")
	f.StringVar(&c.metadataDir, "d", "", "local directory in which to store metadata")
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
	if err == tools.ErrNoTools && !c.noS3 {
		sourceStorage = ec2.NewHTTPStorageReader(sync.DefaultToolsLocation)
		toolsList, err = tools.ReadList(sourceStorage, version.Current.Major, minorVersion)
	}
	if err != nil {
		return err
	}

	targetStorage, err := filestorage.NewFileStorageWriter(c.metadataDir, "")
	if err != nil {
		return err
	}
	return tools.WriteMetadata(toolsList, c.fetch, targetStorage)
}
