// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage

import (
	"context"
	"regexp"

	"github.com/juju/errors"
	"github.com/juju/gnuflag"
	"github.com/juju/names/v6"

	apistorage "github.com/juju/juju/api/client/storage"
	"github.com/juju/juju/api/jujuclient"
	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/cmd"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/core/storage"
	internalstorage "github.com/juju/juju/internal/storage"
)

// NewImportFilesystemCommand returns a command used to import a filesystem.
//
// newStorageImporter is the function to use to acquire a StorageImporter.
// A non-nil function must be provided.
//
// store is an optional ClientStore to use for interacting with the client
// model/controller storage. If nil, the default file-based store will be
// used.
func NewImportFilesystemCommand(
	newStorageImporter NewStorageImporterFunc,
	store jujuclient.ClientStore,
) cmd.Command {
	cmd := &importFilesystemCommand{}
	cmd.newAPIFunc = newStorageImporter
	if store != nil {
		cmd.SetClientStore(store)
	}
	return modelcmd.Wrap(cmd)
}

// NewStorageImporterFunc is the type of a function passed to
// NewImportFilesystemCommand, in order to acquire a StorageImporter.
type NewStorageImporterFunc func(context.Context, *StorageCommandBase) (StorageImporter, error)

// NewStorageImporter returns a new StorageImporter,
// given a StorageCommandBase.
func NewStorageImporter(ctx context.Context, cmd *StorageCommandBase) (StorageImporter, error) {
	api, err := cmd.NewStorageAPI(ctx)
	return apiStorageImporter{Client: api}, err
}

const (
	importFilesystemCommandDoc = `
Adopt an existing filesystem into this model. Supply the name of an existing
storage pool, the filesystem's provider ID (or its backing volume's provider
ID), and a filesystem storage name accepted by the intended charm. Import
reports a new, detached Juju instance ID. It does not copy data or attach it.

Check that the filesystem is free for reuse. After a failed import, inspect
the provider object before retrying. On Kubernetes, check the volume and its claim.

Further reading:

- https://canonical.com/juju/docs/juju-cli/latest/reference/storage/
`
	importFilesystemCommandExamples = `
Import an unused EBS volume containing a filesystem for the pgdata storage name:

    juju import-filesystem ebs vol-123456 pgdata

Import the unbound Kubernetes PersistentVolume pv-data-001 after checking
its reclaim policy is Retain and that no other workload uses it:

    juju import-filesystem kubernetes pv-data-001 pgdata
`

	importFilesystemCommandAgs = `<pool> <provider-id> <storage-name>`
)

// importFilesystemCommand imports filesystems into the model.
type importFilesystemCommand struct {
	StorageCommandBase
	newAPIFunc NewStorageImporterFunc

	storagePool       string
	storageProviderId string
	storageName       string
	force             bool
}

// Init implements Command.Init.
func (c *importFilesystemCommand) Init(args []string) error {
	if len(args) < 3 {
		return errors.New("import-filesystem requires a storage provider, provider ID, and storage name")
	}
	c.storagePool = args[0]
	c.storageProviderId = args[1]
	c.storageName = args[2]

	if !storage.IsValidPoolName(c.storagePool) {
		return errors.NotValidf("pool name %q", c.storagePool)
	}

	validStorageName, err := regexp.MatchString(names.StorageNameSnippet, c.storageName)
	if err != nil {
		return errors.Trace(err)
	}
	if !validStorageName {
		return errors.Errorf("%q is not a valid storage name", c.storageName)
	}
	return nil
}

// SetFlags implements Command.SetFlags.
func (c *importFilesystemCommand) SetFlags(f *gnuflag.FlagSet) {
	c.StorageCommandBase.SetFlags(f)
	f.BoolVar(&c.force, "force", false, "Request provider-specific forced import; ownership checks still apply")
}

// Info implements Command.Info.
func (c *importFilesystemCommand) Info() *cmd.Info {
	return jujucmd.Info(&cmd.Info{
		Name:     "import-filesystem",
		Purpose:  "Imports a filesystem into the model.",
		Doc:      importFilesystemCommandDoc,
		Args:     importFilesystemCommandAgs,
		Examples: importFilesystemCommandExamples,
		SeeAlso:  []string{"storage"},
	})
}

// Run implements Command.Run.
func (c *importFilesystemCommand) Run(ctx *cmd.Context) (err error) {
	api, err := c.newAPIFunc(ctx, &c.StorageCommandBase)
	if err != nil {
		return err
	}
	defer api.Close()

	ctx.Infof(
		"importing %q from storage pool %q as storage %q",
		c.storageProviderId, c.storagePool, c.storageName,
	)
	storageTag, err := api.ImportStorage(
		ctx,
		internalstorage.StorageKindFilesystem,
		c.storagePool, c.storageProviderId, c.storageName, c.force,
	)
	if err != nil {
		return err
	}
	ctx.Infof("imported storage %s", storageTag.Id())
	return nil
}

// StorageImporter provides a method for importing storage into the model.
type StorageImporter interface {
	Close() error

	ImportStorage(
		ctx context.Context,
		kind internalstorage.StorageKind,
		storagePool, storageProviderId, storageName string,
		force bool,
	) (names.StorageTag, error)
}

type apiStorageImporter struct {
	*apistorage.Client
}

func (a apiStorageImporter) ImportStorage(
	ctx context.Context,
	kind internalstorage.StorageKind, storagePool, storageProviderId, storageName string, force bool,
) (names.StorageTag, error) {
	return a.Import(ctx, kind, storagePool, storageProviderId, storageName, force)
}
