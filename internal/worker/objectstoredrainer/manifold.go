// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package objectstoredrainer

import (
	"context"
	"fmt"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/dependency"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/controller"
	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/model"
	coreobjectstore "github.com/juju/juju/core/objectstore"
	"github.com/juju/juju/internal/services"
	"github.com/juju/juju/internal/worker/fortress"
)

// ObjectStoreService is the interface that is used to get a object store.
type ObjectStoreService interface {
	ObjectStore() coreobjectstore.ObjectStoreMetadata
}

// ObjectStoreServicesGetter is the interface that is used to get a object store
// service for a given model UUID.
type ObjectStoreServicesGetter interface {
	// ServicesForModel returns the object store services for the given model UUID.
	ServicesForModel(modelUUID model.UUID) ObjectStoreService
}

// GetObjectStoreServiceServicesFunc is a function that retrieves the
// object store services from the dependency getter.
type GetObjectStoreServiceServicesFunc func(dependency.Getter, string) (ObjectStoreServicesGetter, error)

// GetGuardServiceFunc is a function that retrieves the
// controller object store services from the dependency getter.
type GetGuardServiceFunc func(dependency.Getter, string) (GuardService, error)

// GetControllerConfigServiceFunc is a helper function that gets a service from
// the manifold.
type GetControllerConfigServiceFunc func(getter dependency.Getter, name string) (ControllerConfigService, error)

// ControllerConfigService is the interface that the worker uses to get the
// controller configuration.
type ControllerConfigService interface {
	// ControllerConfig returns the current controller configuration.
	ControllerConfig(context.Context) (controller.Config, error)
}

// ManifoldConfig holds the dependencies and configuration for a
// Worker manifold.
type ManifoldConfig struct {
	AgentName               string
	ObjectStoreServicesName string
	FortressName            string
	S3ClientName            string

	GeObjectStoreServices      GetObjectStoreServiceServicesFunc
	GetGuardService            GetGuardServiceFunc
	GetControllerConfigService GetControllerConfigServiceFunc
	NewWorker                  func(Config) (worker.Worker, error)

	Logger logger.Logger
	Clock  clock.Clock
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
	if config.S3ClientName == "" {
		return errors.NotValidf("empty S3ClientName")
	}
	if config.GeObjectStoreServices == nil {
		return errors.NotValidf("nil GeObjectStoreServices")
	}
	if config.GetGuardService == nil {
		return errors.NotValidf("nil GetGuardService")
	}
	if config.NewWorker == nil {
		return errors.NotValidf("nil NewWorker")
	}
	if config.Logger == nil {
		return errors.NotValidf("nil Logger")
	}
	if config.Clock == nil {
		return errors.NotValidf("nil Clock")
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

	controllerConfigService, err := config.GetControllerConfigService(getter, config.ObjectStoreServicesName)
	if err != nil {
		return nil, errors.Trace(err)
	}

	guardService, err := config.GetGuardService(getter, config.ObjectStoreServicesName)
	if err != nil {
		return nil, errors.Trace(err)
	}

	objectStoreServicesGetter, err := config.GeObjectStoreServices(getter, config.ObjectStoreServicesName)
	if err != nil {
		return nil, errors.Trace(err)
	}

	var fortress fortress.Guard
	if err := getter.Get(config.FortressName, &fortress); err != nil {
		return nil, errors.Trace(err)
	}

	var s3Client coreobjectstore.Client
	if err := getter.Get(config.S3ClientName, &s3Client); err != nil {
		return nil, errors.Trace(err)
	}

	controllerConfig, err := controllerConfigService.ControllerConfig(ctx)
	if err != nil {
		return nil, errors.Trace(err)
	}
	rootBucketName, err := bucketName(controllerConfig)
	if err != nil {
		return nil, errors.Trace(err)
	}

	dataDir := a.CurrentConfig().DataDir()

	worker, err := config.NewWorker(Config{
		Guard:                     fortress,
		GuardService:              guardService,
		ObjectStoreServicesGetter: objectStoreServicesGetter,
		S3Client:                  s3Client,
		RootDir:                   dataDir,
		RootBucketName:            rootBucketName,
		Logger:                    config.Logger,
		Clock:                     config.Clock,
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
			config.S3ClientName,
		},
		Start: config.start,
	}
}

// GetObjectStoreServices retrieves the ObjectStoreService using the given
// service.
func GeObjectStoreServices(getter dependency.Getter, name string) (ObjectStoreServicesGetter, error) {
	var services services.ObjectStoreServicesGetter
	if err := getter.Get(name, &services); err != nil {
		return nil, errors.Trace(err)
	}

	return modelMetadataServiceGetter{
		servicesGetter: services,
	}, nil
}

func GetControllerObjectStoreServices(getter dependency.Getter, name string) (GuardService, error) {
	var services services.ControllerObjectStoreServices
	if err := getter.Get(name, &services); err != nil {
		return nil, errors.Trace(err)
	}

	return services.AgentObjectStore(), nil
}

func bucketName(config controller.Config) (string, error) {
	name := fmt.Sprintf("juju-%s", config.ControllerUUID())
	if _, err := coreobjectstore.ParseObjectStoreBucketName(name); err != nil {
		return "", errors.Trace(err)
	}
	return name, nil
}

type modelMetadataServiceGetter struct {
	servicesGetter services.ObjectStoreServicesGetter
}

// ForModelUUID returns the MetadataService for the given model UUID.
func (s modelMetadataServiceGetter) ServicesForModel(modelUUID model.UUID) ObjectStoreService {
	return modelMetadataService{factory: s.servicesGetter.ServicesForModel(modelUUID)}
}

type modelMetadataService struct {
	factory services.ObjectStoreServices
}

// ObjectStore returns the object store metadata for the given model UUID
func (s modelMetadataService) ObjectStore() coreobjectstore.ObjectStoreMetadata {
	return s.factory.ObjectStore()
}
