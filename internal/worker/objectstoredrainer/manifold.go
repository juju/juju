// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package objectstoredrainer

import (
	"context"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/dependency"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/core/logger"
	coreobjectstore "github.com/juju/juju/core/objectstore"
	"github.com/juju/juju/internal/objectstore"
	"github.com/juju/juju/internal/objectstore/remote"
	"github.com/juju/juju/internal/services"
	"github.com/juju/juju/internal/worker/apiremotecaller"
	"github.com/juju/juju/internal/worker/fortress"
	"github.com/juju/juju/internal/worker/lease"
)

// GetObjectStoreServiceServicesFunc is a function that retrieves the
// object store services from the dependency getter.
type GetObjectStoreServiceServicesFunc func(dependency.Getter, string) (ObjectStoreService, error)

// ManifoldConfig holds the dependencies and configuration for a
// Worker manifold.
type ManifoldConfig struct {
	AgentName               string
	ObjectStoreServicesName string
	FortressName            string
	LeaseManagerName        string
	S3ClientName            string

	GeObjectStoreServicesFn GetObjectStoreServiceServicesFunc
	NewWorker               func(context.Context, Config) (worker.Worker, error)

	NewFilesystemObjectStoreWorker objectstore.ObjectStoreWorkerFunc
	NewS3ObjectStoreWorker         objectstore.ObjectStoreWorkerFunc

	Clock  clock.Clock
	Logger logger.Logger
}

// validate is called by start to check for bad configuration.
func (config ManifoldConfig) Validate() error {
	if config.AgentName == "" {
		return errors.NotValidf("empty AgentName")
	}
	if config.FortressName == "" {
		return errors.NotValidf("empty FortressName")
	}
	if config.ObjectStoreServicesName == "" {
		return errors.NotValidf("empty ObjectStoreServicesName")
	}
	if config.GeObjectStoreServicesFn == nil {
		return errors.NotValidf("nil GeObjectStoreServicesFn")
	}
	if config.NewFilesystemObjectStoreWorker == nil {
		return errors.NotValidf("nil NewFilesystemObjectStoreWorker")
	}
	if config.NewS3ObjectStoreWorker == nil {
		return errors.NotValidf("nil NewS3ObjectStoreWorker")
	}
	if config.NewWorker == nil {
		return errors.NotValidf("nil NewWorker")
	}
	if config.LeaseManagerName == "" {
		return errors.NotValidf("empty LeaseManagerName")
	}
	if config.S3ClientName == "" {
		return errors.NotValidf("empty S3ClientName")
	}
	if config.Clock == nil {
		return errors.NotValidf("nil Clock")
	}
	if config.Logger == nil {
		return errors.NotValidf("nil Logger")
	}
	return nil
}

// start is a StartFunc for a Worker manifold.
func (config ManifoldConfig) start(ctx context.Context, getter dependency.Getter) (worker.Worker, error) {
	if err := config.Validate(); err != nil {
		return nil, errors.Trace(err)
	}

	var a agent.Agent
	if err := getter.Get(config.AgentName, &a); err != nil {
		return nil, err
	}

	objectStoreService, err := config.GeObjectStoreServicesFn(getter, config.ObjectStoreServicesName)
	if err != nil {
		return nil, errors.Trace(err)
	}

	var fortress fortress.Guard
	if err := getter.Get(config.FortressName, &fortress); err != nil {
		return nil, errors.Trace(err)
	}

	var leaseManager lease.Manager
	if err := getter.Get(config.LeaseManagerName, &leaseManager); err != nil {
		return nil, errors.Trace(err)
	}

	var s3Client coreobjectstore.Client
	if err := getter.Get(config.S3ClientName, &s3Client); err != nil {
		return nil, errors.Trace(err)
	}

	worker, err := config.NewWorker(ctx, Config{
		Guard:              fortress,
		ObjectStoreService: objectStoreService,
		LeaseManager:       leaseManager,
		S3Client:           s3Client,
		Clock:              config.Clock,
		Logger:             config.Logger,
	})
	if err != nil {
		return nil, errors.Trace(err)
	}
	return worker, nil
}

// Manifold packages a Worker for use in a dependency.Engine.
func Manifold(config ManifoldConfig) dependency.Manifold {
	return dependency.Manifold{
		Inputs: []string{
			config.AgentName,
			config.FortressName,
			config.ObjectStoreServicesName,
			config.LeaseManagerName,
			config.S3ClientName,
		},
		Start: config.start,
	}
}

// GetObjectStoreServices retrieves the ObjectStoreService using the given
// service.
func GeObjectStoreServices(getter dependency.Getter, name string) (ObjectStoreService, error) {
	var services services.ControllerObjectStoreServices
	if err := getter.Get(name, &services); err != nil {
		return nil, errors.Trace(err)
	}

	return services.AgentObjectStore(), nil
}

// NewFilesystemObjectStoreWorker creates a new filesystem object store worker
// with the given namespace and options.
func NewFilesystemObjectStoreWorker(ctx context.Context,
	namespace, rootDir string,
	claimer objectstore.Claimer,
	metadataService objectstore.MetadataService,
	apiRemoteCallers apiremotecaller.APIRemoteCallers,
	remoteBlobsClient remote.NewBlobsClientFunc,
	logger logger.Logger,
	clock clock.Clock,
) (objectstore.TrackedObjectStore, error) {
	return objectstore.ObjectStoreFactory(ctx, coreobjectstore.FileBackend, namespace,
		objectstore.WithRootDir(rootDir),
		objectstore.WithMetadataService(metadataService),
		objectstore.WithAPIRemoveCallers(apiRemoteCallers),
		objectstore.WithNewBlobsClient(remoteBlobsClient),
		objectstore.WithClaimer(claimer),
		objectstore.WithLogger(logger),
		objectstore.WithClock(clock),
	)
}

// NewS3ObjectStoreWorker creates a new S3 object store worker with the given
// namespace and options.
func NewS3ObjectStoreWorker(
	ctx context.Context,
	namespace, rootBucket, rootDir string,
	metadataService objectstore.MetadataService,
	s3client coreobjectstore.Client,
	claimer objectstore.Claimer,
	logger logger.Logger,
	clock clock.Clock,
) (objectstore.TrackedObjectStore, error) {
	return objectstore.ObjectStoreFactory(ctx, coreobjectstore.S3Backend, namespace,
		objectstore.WithS3Client(s3client),
		objectstore.WithRootBucket(rootBucket),
		objectstore.WithRootDir(rootDir),
		objectstore.WithMetadataService(metadataService),
		objectstore.WithClaimer(claimer),
		objectstore.WithLogger(logger),
		objectstore.WithClock(clock),
	)
}
