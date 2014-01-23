// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"fmt"
	"net/url"
	"path"
	"strings"

	"launchpad.net/gnuflag"
	"launchpad.net/loggo"

	"launchpad.net/juju-core/cmd"
	"launchpad.net/juju-core/environs/filestorage"
	"launchpad.net/juju-core/environs/simplestreams"
	"launchpad.net/juju-core/environs/storage"
	envtools "launchpad.net/juju-core/environs/tools"
	"launchpad.net/juju-core/juju/osenv"
	coretools "launchpad.net/juju-core/tools"
	"launchpad.net/juju-core/utils"
	"launchpad.net/juju-core/version"
)

// ToolsMetadataCommand is used to generate simplestreams metadata for juju tools.
type ToolsMetadataCommand struct {
	cmd.EnvCommandBase
	fetch       bool
	metadataDir string
	public      bool
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
		c.metadataDir = osenv.JujuHome()
	}
	var err error
	c.metadataDir, err = utils.NormalizePath(c.metadataDir)
	if err != nil {
		return err
	}

	sourceStorage, err := filestorage.NewFileStorageReader(c.metadataDir)
	if err != nil {
		return err
	}
	fmt.Fprintln(context.Stdout, "Finding tools...")
	const minorVersion = -1
	toolsList, err := envtools.ReadList(sourceStorage, version.Current.Major, minorVersion)
	if err == envtools.ErrNoTools {
		source := envtools.DefaultBaseURL
		var u *url.URL
		u, err = url.Parse(source)
		if err != nil {
			return fmt.Errorf("invalid tools source %s: %v", source, err)
		}
		if u.Scheme == "" {
			source = "file://" + source
			if !strings.HasSuffix(source, "/"+storage.BaseToolsPath) {
				source = path.Join(source, storage.BaseToolsPath)
			}
		}
		sourceDataSource := simplestreams.NewURLDataSource(source, simplestreams.VerifySSLHostnames)
		toolsList, err = envtools.FindToolsForCloud(
			[]simplestreams.DataSource{sourceDataSource}, simplestreams.CloudSpec{},
			version.Current.Major, minorVersion, coretools.Filter{})
	}
	if err != nil {
		return err
	}

	targetStorage, err := filestorage.NewFileStorageWriter(c.metadataDir, filestorage.UseDefaultTmpDir)
	if err != nil {
		return err
	}
	writeMirrors := envtools.DoNotWriteMirrors
	if c.public {
		writeMirrors = envtools.WriteMirrors
	}
	return mergeAndWriteMetadata(targetStorage, toolsList, writeMirrors)
}

// This is essentially the same as tools.MergeAndWriteMetadata, but also
// resolves metadata for existing tools by fetching them and computing
// size/sha256 locally.
func mergeAndWriteMetadata(stor storage.Storage, toolsList coretools.List, writeMirrors envtools.ShouldWriteMirrors) error {
	existing, err := envtools.ReadMetadata(stor)
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
	return envtools.WriteMetadata(stor, metadata, writeMirrors)
}
