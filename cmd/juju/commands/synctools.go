// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package commands

import (
	"bytes"
	"io"

	"github.com/juju/cmd"
	"github.com/juju/loggo"
	"launchpad.net/gnuflag"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/cmd/envcmd"
	"github.com/juju/juju/cmd/juju/block"
	"github.com/juju/juju/environs/filestorage"
	"github.com/juju/juju/environs/sync"
	envtools "github.com/juju/juju/environs/tools"
	coretools "github.com/juju/juju/tools"
	"github.com/juju/juju/version"
)

var syncTools = sync.SyncTools

// SyncToolsCommand copies all the tools from the us-east-1 bucket to the local
// bucket.
type SyncToolsCommand struct {
	envcmd.EnvCommandBase
	allVersions  bool
	versionStr   string
	majorVersion int
	minorVersion int
	dryRun       bool
	dev          bool
	public       bool
	source       string
	stream       string
	localDir     string
	destination  string
}

var _ cmd.Command = (*SyncToolsCommand)(nil)

func (c *SyncToolsCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "sync-tools",
		Purpose: "copy tools from the official tool store into a local environment",
		Doc: `
This copies the Juju tools tarball from the official tools store (located
at https://streams.canonical.com/juju) into your environment.
This is generally done when you want Juju to be able to run without having to
access the Internet. Alternatively you can specify a local directory as source.

Sometimes this is because the environment does not have public access,
and sometimes you just want to avoid having to access data outside of
the local cloud.
`,
	}
}

func (c *SyncToolsCommand) SetFlags(f *gnuflag.FlagSet) {
	f.BoolVar(&c.allVersions, "all", false, "copy all versions, not just the latest")
	f.StringVar(&c.versionStr, "version", "", "copy a specific major[.minor] version")
	f.BoolVar(&c.dryRun, "dry-run", false, "don't copy, just print what would be copied")
	f.BoolVar(&c.dev, "dev", false, "consider development versions as well as released ones\n    DEPRECATED: use --stream instead")
	f.BoolVar(&c.public, "public", false, "tools are for a public cloud, so generate mirrors information")
	f.StringVar(&c.source, "source", "", "local source directory")
	f.StringVar(&c.stream, "stream", "", "simplestreams stream for which to sync metadata")
	f.StringVar(&c.localDir, "local-dir", "", "local destination directory")
	f.StringVar(&c.destination, "destination", "", "local destination directory")
}

func (c *SyncToolsCommand) Init(args []string) error {
	if c.destination != "" {
		// Override localDir with destination as localDir now replaces destination
		c.localDir = c.destination
		logger.Warningf("Use of the --destination flag is deprecated in 1.18. Please use --local-dir instead.")
	}
	if c.versionStr != "" {
		var err error
		if c.majorVersion, c.minorVersion, err = version.ParseMajorMinor(c.versionStr); err != nil {
			return err
		}
	}
	if c.dev {
		c.stream = envtools.TestingStream
	}
	return cmd.CheckEmpty(args)
}

// syncToolsAPI provides an interface with a subset of the
// api.Client API. This exists to enable mocking.
type syncToolsAPI interface {
	FindTools(majorVersion, minorVersion int, series, arch string) (params.FindToolsResult, error)
	UploadTools(r io.Reader, v version.Binary, series ...string) (*coretools.Tools, error)
	Close() error
}

var getSyncToolsAPI = func(c *SyncToolsCommand) (syncToolsAPI, error) {
	return c.NewAPIClient()
}

func (c *SyncToolsCommand) Run(ctx *cmd.Context) (resultErr error) {
	// Register writer for output on screen.
	loggo.RegisterWriter("synctools", cmd.NewCommandLogWriter("juju.environs.sync", ctx.Stdout, ctx.Stderr), loggo.INFO)
	defer loggo.RemoveWriter("synctools")

	sctx := &sync.SyncContext{
		AllVersions:  c.allVersions,
		MajorVersion: c.majorVersion,
		MinorVersion: c.minorVersion,
		DryRun:       c.dryRun,
		Stream:       c.stream,
		Source:       c.source,
	}

	if c.localDir != "" {
		stor, err := filestorage.NewFileStorageWriter(c.localDir)
		if err != nil {
			return err
		}
		writeMirrors := envtools.DoNotWriteMirrors
		if c.public {
			writeMirrors = envtools.WriteMirrors
		}
		sctx.TargetToolsFinder = sync.StorageToolsFinder{Storage: stor}
		sctx.TargetToolsUploader = sync.StorageToolsUploader{
			Storage:       stor,
			WriteMetadata: true,
			WriteMirrors:  writeMirrors,
		}
	} else {
		if c.public {
			logger.Warningf("--public is ignored unless --local-dir is specified")
		}
		api, err := getSyncToolsAPI(c)
		if err != nil {
			return err
		}
		defer api.Close()
		adapter := syncToolsAPIAdapter{api}
		sctx.TargetToolsFinder = adapter
		sctx.TargetToolsUploader = adapter
	}
	return block.ProcessBlockedError(syncTools(sctx), block.BlockChange)
}

// syncToolsAPIAdapter implements sync.ToolsFinder and
// sync.ToolsUploader, adapting a syncToolsAPI. This
// enables the use of sync.SyncTools with the client
// API.
type syncToolsAPIAdapter struct {
	syncToolsAPI
}

func (s syncToolsAPIAdapter) FindTools(majorVersion int, stream string) (coretools.List, error) {
	result, err := s.syncToolsAPI.FindTools(majorVersion, -1, "", "")
	if err != nil {
		return nil, err
	}
	if result.Error != nil {
		if params.IsCodeNotFound(result.Error) {
			return nil, coretools.ErrNoMatches
		}
		return nil, result.Error
	}
	return result.List, nil
}

func (s syncToolsAPIAdapter) UploadTools(toolsDir, stream string, tools *coretools.Tools, data []byte) error {
	_, err := s.syncToolsAPI.UploadTools(bytes.NewReader(data), tools.Version)
	return err
}
