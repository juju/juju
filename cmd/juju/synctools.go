// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"launchpad.net/gnuflag"
	"launchpad.net/loggo"

	"launchpad.net/juju-core/cmd"
	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/environs/filestorage"
	"launchpad.net/juju-core/environs/sync"
	"launchpad.net/juju-core/version"
)

var syncTools = sync.SyncTools

// SyncToolsCommand copies all the tools from the us-east-1 bucket to the local
// bucket.
type SyncToolsCommand struct {
	cmd.EnvCommandBase
	allVersions  bool
	versionStr   string
	majorVersion int
	minorVersion int
	dryRun       bool
	dev          bool
	source       string
	destination  string
}

var _ cmd.Command = (*SyncToolsCommand)(nil)

func (c *SyncToolsCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "sync-tools",
		Purpose: "copy tools from the official bucket into a local environment",
		Doc: `
This copies the Juju tools tarball from the official bucket into
your environment. This is generally done when you want Juju to be able
to run without having to access Amazon. Alternatively you can specify
a local directory as source.

Sometimes this is because the environment does not have public access,
and sometimes you just want to avoid having to access data outside of
the local cloud.
`,
	}
}

func (c *SyncToolsCommand) SetFlags(f *gnuflag.FlagSet) {
	c.EnvCommandBase.SetFlags(f)
	f.BoolVar(&c.allVersions, "all", false, "copy all versions, not just the latest")
	f.StringVar(&c.versionStr, "version", "", "copy a specific major[.minor] version")
	f.BoolVar(&c.dryRun, "dry-run", false, "don't copy, just print what would be copied")
	f.BoolVar(&c.dev, "dev", false, "consider development versions as well as released ones")
	f.StringVar(&c.source, "source", "", "chose a location on the file system as source")
	f.StringVar(&c.destination, "destination", "", "chose a location on the file system as destination")

	// BUG(lp:1163164)  jam 2013-04-2 we would like to add a "source"
	// location, rather than only copying from us-east-1
}

func (c *SyncToolsCommand) Init(args []string) error {
	if c.versionStr != "" {
		var err error
		if c.majorVersion, c.minorVersion, err = version.ParseMajorMinor(c.versionStr); err != nil {
			return err
		}
	}
	return cmd.CheckEmpty(args)
}

func (c *SyncToolsCommand) Run(ctx *cmd.Context) error {
	// Register writer for output on screen.
	loggo.RegisterWriter("synctools", cmd.NewCommandLogWriter("juju.environs.sync", ctx.Stdout, ctx.Stderr), loggo.INFO)
	defer loggo.RemoveWriter("synctools")
	environ, err := environs.PrepareFromName(c.EnvName)
	if err != nil {
		return err
	}

	target := environ.Storage()
	if c.destination != "" {
		target, err = filestorage.NewFileStorageWriter(c.destination, "")
		if err != nil {
			return err
		}
	}

	// Prepare syncing.
	sctx := &sync.SyncContext{
		Target:       target,
		AllVersions:  c.allVersions,
		MajorVersion: c.majorVersion,
		MinorVersion: c.minorVersion,
		DryRun:       c.dryRun,
		Dev:          c.dev,
		Source:       c.source,
	}
	return syncTools(sctx)
}
