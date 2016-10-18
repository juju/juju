// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package commands

import (
	"bytes"
	"io"

	"github.com/juju/cmd"
	"github.com/juju/gnuflag"
	"github.com/juju/loggo"
	"github.com/juju/version"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/cmd/juju/block"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/environs/filestorage"
	"github.com/juju/juju/environs/sync"
	envtools "github.com/juju/juju/environs/tools"
	coretools "github.com/juju/juju/tools"
)

var syncTools = sync.SyncTools

func newSyncToolsCommand() cmd.Command {
	return modelcmd.Wrap(&syncToolsCommand{})
}

// syncToolsCommand copies all the tools from the us-east-1 bucket to the local
// bucket.
type syncToolsCommand struct {
	modelcmd.ModelCommandBase
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

var _ cmd.Command = (*syncToolsCommand)(nil)

const synctoolsDoc = `
This copies the Juju agent software from the official tools store (located
at https://streams.canonical.com/juju) into a model. It is generally done
when the model is without Internet access.

Instead of the above site, a local directory can be specified as source.
The online store will, of course, need to be contacted at some point to get
the software.

Examples:
    # Download the software (version auto-selected) to the model:
    juju sync-tools --debug

    # Download a specific version of the software locally:
    juju sync-tools --debug --version 2.0 --local-dir=/home/ubuntu/sync-tools

    # Get locally available software to the model:
    juju sync-tools --debug --source=/home/ubuntu/sync-tools

See also:
    upgrade-juju

`

func (c *syncToolsCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "sync-tools",
		Purpose: "Copy tools from the official tool store into a local model.",
		Doc:     synctoolsDoc,
	}
}

func (c *syncToolsCommand) SetFlags(f *gnuflag.FlagSet) {
	c.ModelCommandBase.SetFlags(f)
	f.BoolVar(&c.allVersions, "all", false, "Copy all versions, not just the latest")
	f.StringVar(&c.versionStr, "version", "", "Copy a specific major[.minor] version")
	f.BoolVar(&c.dryRun, "dry-run", false, "Don't copy, just print what would be copied")
	f.BoolVar(&c.dev, "dev", false, "Consider development versions as well as released ones\n    DEPRECATED: use --stream instead")
	f.BoolVar(&c.public, "public", false, "Tools are for a public cloud, so generate mirrors information")
	f.StringVar(&c.source, "source", "", "Local source directory")
	f.StringVar(&c.stream, "stream", "", "Simplestreams stream for which to sync metadata")
	f.StringVar(&c.localDir, "local-dir", "", "Local destination directory")
	f.StringVar(&c.destination, "destination", "", "Local destination directory\n    DEPRECATED: use --local-dir instead")
}

func (c *syncToolsCommand) Init(args []string) error {
	if c.destination != "" {
		// Override localDir with destination as localDir now replaces destination
		c.localDir = c.destination
		logger.Infof("Use of the --destination flag is deprecated in 1.18. Please use --local-dir instead.")
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
	UploadTools(r io.ReadSeeker, v version.Binary, series ...string) (coretools.List, error)
	Close() error
}

var getSyncToolsAPI = func(c *syncToolsCommand) (syncToolsAPI, error) {
	return c.NewAPIClient()
}

func (c *syncToolsCommand) Run(ctx *cmd.Context) (resultErr error) {
	// Register writer for output on screen.
	writer := loggo.NewMinimumLevelWriter(
		cmd.NewCommandLogWriter("juju.environs.sync", ctx.Stdout, ctx.Stderr),
		loggo.INFO)
	loggo.RegisterWriter("synctools", writer)
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
			logger.Infof("--public is ignored unless --local-dir is specified")
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
