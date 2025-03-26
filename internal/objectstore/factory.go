// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package objectstore

import (
	"context"

	"github.com/juju/clock"
	jujuerrors "github.com/juju/errors"
	"github.com/juju/worker/v4"

	"github.com/juju/juju/controller"
	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/objectstore"
	"github.com/juju/juju/internal/errors"
	internallogger "github.com/juju/juju/internal/logger"
	"github.com/juju/juju/internal/objectstore/remote"
	"github.com/juju/juju/internal/worker/apiremotecaller"
)

// MetadataService is the interface that is used to get a object store.
type MetadataService interface {
	ObjectStore() objectstore.ObjectStoreMetadata
}

// TrackedObjectStore is a ObjectStore that is also a worker, to ensure the
// lifecycle of the objectStore is managed.
type TrackedObjectStore interface {
	worker.Worker
	objectstore.ObjectStore
}

// Option is the function signature for the options to create a new object
// store.
type Option func(*options)

// WithRootDir is the option to set the root directory to use.
func WithRootDir(rootDir string) Option {
	return func(o *options) {
		o.rootDir = rootDir
	}
}

// WithMetadataService is the option to set the metadata service to use.
func WithMetadataService(metadataService MetadataService) Option {
	return func(o *options) {
		o.metadataService = metadataService
	}
}

// WithLogger is the option to set the logger to use.
func WithLogger(logger logger.Logger) Option {
	return func(o *options) {
		o.logger = logger
	}
}

// WithClaimer is the option to set the claimer to use.
func WithClaimer(claimer Claimer) Option {
	return func(o *options) {
		o.claimer = claimer
	}
}

// WithClock is the option to set the clock to use.
func WithClock(clock clock.Clock) Option {
	return func(o *options) {
		o.clock = clock
	}
}

// WithAllowDraining is the option to set the allow draining to use.
// This is for s3 base object stores.
func WithAllowDraining(allowDraining bool) Option {
	return func(o *options) {
		o.allowDraining = allowDraining
	}
}

// WithRootBucket is the option to set the root bucket to use.
// This is for s3 base object stores.
func WithRootBucket(rootBucket string) Option {
	return func(o *options) {
		o.rootBucket = rootBucket
	}
}

// WithS3Client is the option to set the s3 client to use.
// This is for s3 base object stores.
func WithS3Client(client objectstore.Client) Option {
	return func(o *options) {
		o.s3Client = client
	}
}

// WithAPIRemoveCallers is the option to set the api remote callers to use.
// The default if not set is to return no API remotes, which will always be
// local only requests.
// This is for file based object stores.
func WithAPIRemoveCallers(apiRemoteCallers apiremotecaller.APIRemoteCallers) Option {
	return func(o *options) {
		o.apiRemoteCallers = apiRemoteCallers
	}
}

// WithNewBlobsClient is the option to set the new blobs client for file
// retrieval from another controller.
// This is for file based object stores.
func WithNewBlobsClient(clientFn remote.NewBlobsClientFunc) Option {
	return func(o *options) {
		o.newFileBlobsClient = clientFn
	}
}

type options struct {
	rootDir         string
	claimer         Claimer
	metadataService MetadataService
	logger          logger.Logger
	clock           clock.Clock

	// S3 base options
	allowDraining bool
	rootBucket    string
	s3Client      objectstore.Client

	// File base options
	apiRemoteCallers   apiremotecaller.APIRemoteCallers
	newFileBlobsClient remote.NewBlobsClientFunc
}

func newOptions() *options {
	return &options{
		newFileBlobsClient: remote.NewBlobsClient,
		apiRemoteCallers:   noopAPIRemoteCallers{},
		logger:             internallogger.GetLogger("juju.objectstore", logger.OBJECTSTORE),
		clock:              clock.WallClock,
	}
}

// ObjectStoreWorkerFunc is the function signature for creating a new object
// store worker.
type ObjectStoreWorkerFunc func(context.Context, objectstore.BackendType, string, ...Option) (TrackedObjectStore, error)

// ObjectStoreFactory is the function to create a new object store based on
// the backend type.
func ObjectStoreFactory(ctx context.Context, backendType objectstore.BackendType, namespace string, options ...Option) (TrackedObjectStore, error) {
	opts := newOptions()
	for _, option := range options {
		option(opts)
	}

	switch backendType {
	case objectstore.FileBackend:
		blobRetriever, err := remote.NewBlobRetriever(
			opts.apiRemoteCallers,
			namespace,
			opts.newFileBlobsClient,
			opts.clock,
			opts.logger,
		)
		if err != nil {
			return nil, errors.Errorf("creating blob retriever: %w", err)
		}

		fileStore, err := NewFileObjectStore(FileObjectStoreConfig{
			Namespace:       namespace,
			RootDir:         opts.rootDir,
			MetadataService: opts.metadataService.ObjectStore(),
			Claimer:         opts.claimer,
			Logger:          opts.logger,
			Clock:           opts.clock,
			RemoteRetriever: blobRetriever,
		})
		if err != nil {
			return nil, errors.Errorf("creating file based objectstore: %w", err)
		}

		return newRemoteFileObjectStore(fileStore, blobRetriever)

	case objectstore.S3Backend:
		return NewS3ObjectStore(S3ObjectStoreConfig{
			RootBucket:      opts.rootBucket,
			Namespace:       namespace,
			RootDir:         opts.rootDir,
			Client:          opts.s3Client,
			MetadataService: opts.metadataService.ObjectStore(),
			Claimer:         opts.claimer,
			Logger:          opts.logger,
			Clock:           opts.clock,
			AllowDraining:   opts.allowDraining,

			HashFileSystemAccessor: newHashFileSystemAccessor(namespace, opts.rootDir, opts.logger),
		})
	default:
		return nil, errors.Errorf("backend type %q: %w", backendType, jujuerrors.NotValid)
	}
}

// BackendTypeOrDefault returns the default backend type for the given object
// store type or falls back to the default backend type.
func BackendTypeOrDefault(objectStoreType objectstore.BackendType) objectstore.BackendType {
	if s, err := objectstore.ParseObjectStoreType(objectStoreType.String()); err == nil {
		return s
	}
	return DefaultBackendType()
}

// DefaultBackendType returns the default backend type for the given object
// store type or falls back to the default backend type.
func DefaultBackendType() objectstore.BackendType {
	return controller.DefaultObjectStoreType
}
