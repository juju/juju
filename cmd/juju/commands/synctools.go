// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package commands

import (
	"bytes"
	"io"

	"github.com/juju/cmd/v3"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"
	"github.com/juju/loggo"
	"github.com/juju/version/v2"

	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/juju/block"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/environs/filestorage"
	"github.com/juju/juju/environs/sync"
	envtools "github.com/juju/juju/environs/tools"
	"github.com/juju/juju/rpc/params"
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
	modelcmd.IAASOnlyCommand
	allVersions  bool
	versionStr   string
	majorVersion int
	minorVersion int
	dryRun       bool
	public       bool
	source       string
	stream       string
	localDir     string
}

var _ cmd.Command = (*syncToolsCommand)(nil)

const synctoolsDoc = `
This copies the Juju agent software from the official agent binaries store 
(located at https://streams.canonical.com/juju) into the controller.
It is generally done when the controller is without Internet access.

Instead of the above site, a local directory can be specified as source.
The online store will, of course, need to be contacted at some point to get
the software.

Examples:
    juju sync-agent-binaries --debug
    juju sync-agent-binaries --debug --version 2.0 --local-dir=/home/ubuntu/sync-agent-binaries
    juju sync-agent-binaries --debug --source=/home/ubuntu/sync-agent-binaries

See also:
    upgrade-controller

`

func (c *syncToolsCommand) Info() *cmd.Info {
	return jujucmd.Info(&cmd.Info{
		Name:    "sync-agent-binaries",
		Purpose: "Copy agent binaries from the official agent store into a local controller.",
		Doc:     synctoolsDoc,
		Aliases: []string{"sync-tools"},
	})
}

func (c *syncToolsCommand) SetFlags(f *gnuflag.FlagSet) {
	c.ModelCommandBase.SetFlags(f)
	f.BoolVar(&c.allVersions, "all", false, "Copy all versions, not just the latest")
	f.StringVar(&c.versionStr, "version", "", "Copy a specific major[.minor] version")
	f.BoolVar(&c.dryRun, "dry-run", false, "Don't copy, just print what would be copied")
	f.BoolVar(&c.public, "public", false, "Tools are for a public cloud, so generate mirrors information")
	f.StringVar(&c.source, "source", "", "Local source directory")
	f.StringVar(&c.stream, "stream", "", "Simplestreams stream for which to sync metadata")
	f.StringVar(&c.localDir, "local-dir", "", "Local destination directory")
}

func (c *syncToolsCommand) Init(args []string) error {
	if c.versionStr != "" {
		var err error
		if c.majorVersion, c.minorVersion, err = version.ParseMajorMinor(c.versionStr); err != nil {
			return err
		}
	}
	return cmd.CheckEmpty(args)
}

// syncToolsAPI provides an interface with a subset of the
// api.Client API. This exists to enable mocking.
type syncToolsAPI interface {
	FindTools(majorVersion, minorVersion int, osType, arch, agentStream string) (params.FindToolsResult, error)
	UploadTools(r io.ReadSeeker, v version.Binary, _ ...string) (coretools.List, error)
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
	_ = loggo.RegisterWriter("syncagentbinaries", writer)
	defer func() { _, _ = loggo.RemoveWriter("syncagentbinaries") }()

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
	result, err := s.syncToolsAPI.FindTools(majorVersion, -1, "", "", "")
	if errors.IsNotFound(err) {
		return nil, coretools.ErrNoMatches
	}
	if err != nil {
		return nil, err
	}
	return result.List, nil
}

func (s syncToolsAPIAdapter) UploadTools(toolsDir, stream string, tools *coretools.Tools, data []byte) error {
	_, err := s.syncToolsAPI.UploadTools(bytes.NewReader(data), tools.Version)
	return err
}
