// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package commands

import (
	"bytes"
	"context"
	"io"
	"os"

	"github.com/juju/errors"
	"github.com/juju/gnuflag"
	"github.com/juju/loggo/v2"

	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/constants"
	"github.com/juju/juju/cmd/juju/block"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/core/semversion"
	"github.com/juju/juju/environs/filestorage"
	"github.com/juju/juju/environs/sync"
	envtools "github.com/juju/juju/environs/tools"
	"github.com/juju/juju/internal/cmd"
	coretools "github.com/juju/juju/internal/tools"
)

var syncTools = sync.SyncTools

func newSyncAgentBinaryCommand() cmd.Command {
	return modelcmd.Wrap(&syncAgentBinaryCommand{})
}

// syncAgentBinaryCommand copies the tool from either official agent binaries store or
// a local directory to the controller.
type syncAgentBinaryCommand struct {
	modelcmd.ModelCommandBase
	modelcmd.IAASOnlyCommand
	versionStr    string
	targetVersion semversion.Number
	dryRun        bool
	public        bool
	source        string
	stream        string
	localDir      string
	syncToolAPI   SyncToolAPI
}

var _ cmd.Command = (*syncAgentBinaryCommand)(nil)

const synctoolsDoc = `
This copies the Juju agent software from the official agent binaries store 
(located at https://streams.canonical.com/juju) into the controller.
It is generally done when the controller is without Internet access.

Instead of the above site, a local directory can be specified as source.
The online store will, of course, need to be contacted at some point to get
the software.
`

const synctoolsExamples = `
    juju sync-agent-binary --debug --agent-version 2.0
    juju sync-agent-binary --debug --agent-version 2.0 --local-dir=/home/ubuntu/sync-agent-binary
`

func (c *syncAgentBinaryCommand) Info() *cmd.Info {
	return jujucmd.Info(&cmd.Info{
		Name:     "sync-agent-binary",
		Purpose:  "Copy agent binaries from the official agent store into a local controller.",
		Doc:      synctoolsDoc,
		Examples: synctoolsExamples,
		SeeAlso: []string{
			"upgrade-controller",
		},
	})
}

func (c *syncAgentBinaryCommand) SetFlags(f *gnuflag.FlagSet) {
	c.ModelCommandBase.SetFlags(f)
	f.StringVar(&c.versionStr, "agent-version", "", "Copy a specific major[.minor] version")
	f.BoolVar(&c.dryRun, "dry-run", false, "Don't copy, just print what would be copied")
	f.BoolVar(&c.public, "public", false, "Tools are for a public cloud, so generate mirrors information")
	f.StringVar(&c.source, "source", "", "Local source directory")
	f.StringVar(&c.stream, "stream", "", "Simplestreams stream for which to sync metadata")
	f.StringVar(&c.localDir, "local-dir", "", "Local destination directory")
}

func (c *syncAgentBinaryCommand) Init(args []string) error {
	if c.versionStr == "" {
		return errors.NewNotValid(nil, "--agent-version is required")
	}
	var err error
	if c.targetVersion, err = semversion.Parse(c.versionStr); err != nil {
		return err
	}
	return cmd.CheckEmpty(args)
}

// SyncToolAPI provides an interface with a subset of the
// modelupgrader.Client API. This exists to enable mocking.
type SyncToolAPI interface {
	UploadTools(ctx context.Context, r io.Reader, v semversion.Binary) (coretools.List, error)
	Close() error
}

func (c *syncAgentBinaryCommand) getSyncToolAPI(ctx context.Context) (SyncToolAPI, error) {
	if c.syncToolAPI != nil {
		return c.syncToolAPI, nil
	}
	return c.NewModelUpgraderAPIClient(ctx)
}

func (c *syncAgentBinaryCommand) Run(ctx *cmd.Context) (resultErr error) {
	// Register writer for output on screen.
	writer := loggo.NewMinimumLevelWriter(
		cmd.NewCommandLogWriter("juju.environs.sync", ctx.Stdout, ctx.Stderr), loggo.INFO,
	)
	_ = loggo.RegisterWriter("syncagentbinaries", writer)
	defer func() { _, _ = loggo.RemoveWriter("syncagentbinaries") }()

	if envMetadataSrc := os.Getenv(constants.EnvJujuMetadataSource); c.source == "" && envMetadataSrc != "" {
		c.source = envMetadataSrc
		ctx.Infof("Using local simple stream source directory %q", c.source)
	}

	sctx := &sync.SyncContext{
		ChosenVersion: c.targetVersion,
		DryRun:        c.dryRun,
		Stream:        c.stream,
		Source:        c.source,
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
			logger.Infof(context.TODO(), "--public is ignored unless --local-dir is specified")
		}
		api, err := c.getSyncToolAPI(ctx)
		if err != nil {
			return err
		}
		defer api.Close()
		adaptor := syncToolAPIAdaptor{api}
		sctx.TargetToolsUploader = adaptor
	}
	return block.ProcessBlockedError(syncTools(ctx, sctx), block.BlockChange)
}

// syncToolAPIAdaptor implements sync.ToolsFinder and
// sync.ToolsUploader, adapting a syncToolAPI. This
// enables the use of sync.SyncTools.
type syncToolAPIAdaptor struct {
	SyncToolAPI
}

func (s syncToolAPIAdaptor) UploadTools(ctx context.Context, toolsDir, stream string, tools *coretools.Tools, data []byte) error {
	_, err := s.SyncToolAPI.UploadTools(ctx, bytes.NewReader(data), tools.Version)
	return err
}
