// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package objectstore

import (
	"context"
	"fmt"
	"time"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/dependency"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/controller"
	coredependency "github.com/juju/juju/core/dependency"
	"github.com/juju/juju/core/lease"
	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/model"
	coreobjectstore "github.com/juju/juju/core/objectstore"
	"github.com/juju/juju/internal/objectstore"
	"github.com/juju/juju/internal/services"
	"github.com/juju/juju/internal/worker/apiremotecaller"
	"github.com/juju/juju/internal/worker/trace"
)

// MetadataServiceGetter is the interface that is used to get the
// MetadataService for a given model UUID.
type MetadataServiceGetter interface {
	// For returns the MetadataService for the given model UUID.
	ForModelUUID(model.UUID) MetadataService
}

// ModelServiceGetter is the interface that is used to get the ModelService
// for a given model UUID.
type ModelServiceGetter interface {
	// ForModelUUID returns the ModelService for the given model UUID.
	ForModelUUID(model.UUID) ModelServices
}

// ModelServices is the interface that provides model services for a given model
// UUID.
type ModelServices interface {
	// ModelService returns the ModelService for the given model UUID.
	ModelService() ModelService
}

// ModelClaimGetter is the interface that is used to get a model claimer.
type ModelClaimGetter interface {
	ForModelUUID(model.UUID) (objectstore.Claimer, error)
}

// MetadataService is the interface that is used to get a object store.
type MetadataService interface {
	ObjectStore() coreobjectstore.ObjectStoreMetadata
}

// ControllerConfigService is the interface that the worker uses to get the
// controller configuration.
type ControllerConfigService interface {
	// ControllerConfig returns the current controller configuration.
	ControllerConfig(context.Context) (controller.Config, error)
}

// GetControllerConfigServiceFunc is a helper function that gets a service from
// the manifold.
type GetControllerConfigServiceFunc func(getter dependency.Getter, name string) (ControllerConfigService, error)

// GetMetadataServiceFunc is a helper function that gets a service from
// the manifold.
type GetMetadataServiceFunc func(getter dependency.Getter, name string) (MetadataService, error)

// IsBootstrapControllerFunc is a helper function that checks if the controller
// is the initial bootstrap controller.
type IsBootstrapControllerFunc func(dataDir string) bool

// ManifoldConfig defines the configuration for the objectstore manifold.
type ManifoldConfig struct {
	AgentName               string
	TraceName               string
	ObjectStoreServicesName string
	LeaseManagerName        string
	S3ClientName            string
	APIRemoteCallerName     string

	Clock                      clock.Clock
	Logger                     logger.Logger
	NewObjectStoreWorker       objectstore.ObjectStoreWorkerFunc
	GetControllerConfigService GetControllerConfigServiceFunc
	GetMetadataService         GetMetadataServiceFunc
	IsBootstrapController      IsBootstrapControllerFunc
}

// Validate validates the manifold configuration.
func (cfg ManifoldConfig) Validate() error {
	if cfg.AgentName == "" {
		return errors.NotValidf("empty AgentName")
	}
	if cfg.TraceName == "" {
		return errors.NotValidf("empty TraceName")
	}
	if cfg.ObjectStoreServicesName == "" {
		return errors.NotValidf("empty ObjectStoreServicesName")
	}
	if cfg.GetControllerConfigService == nil {
		return errors.NotValidf("nil GetControllerConfigService")
	}
	if cfg.GetMetadataService == nil {
		return errors.NotValidf("nil GetMetadataService")
	}
	if cfg.IsBootstrapController == nil {
		return errors.NotValidf("nil IsBootstrapController")
	}
	if cfg.LeaseManagerName == "" {
		return errors.NotValidf("empty LeaseManagerName")
	}
	if cfg.S3ClientName == "" {
		return errors.NotValidf("empty S3ClientName")
	}
	if cfg.APIRemoteCallerName == "" {
		return errors.NotValidf("empty APIRemoteCallerName")
	}
	if cfg.Clock == nil {
		return errors.NotValidf("nil Clock")
	}
	if cfg.Logger == nil {
		return errors.NotValidf("nil Logger")
	}
	if cfg.NewObjectStoreWorker == nil {
		return errors.NotValidf("nil NewObjectStoreWorker")
	}
	return nil
}

// Manifold returns a dependency manifold that runs the trace worker.
func Manifold(config ManifoldConfig) dependency.Manifold {
	return dependency.Manifold{
		Inputs: []string{
			config.AgentName,
			config.TraceName,
			config.ObjectStoreServicesName,
			config.LeaseManagerName,
			config.S3ClientName,
			config.APIRemoteCallerName,
		},
		Output: output,
		Start: func(ctx context.Context, getter dependency.Getter) (worker.Worker, error) {
			if err := config.Validate(); err != nil {
				return nil, errors.Trace(err)
			}

			var a agent.Agent
			if err := getter.Get(config.AgentName, &a); err != nil {
				return nil, err
			}

			var tracerGetter trace.TracerGetter
			if err := getter.Get(config.TraceName, &tracerGetter); err != nil {
				return nil, errors.Trace(err)
			}

			controllerConfigService, err := config.GetControllerConfigService(getter, config.ObjectStoreServicesName)
			if err != nil {
				return nil, errors.Trace(err)
			}
			metadataService, err := config.GetMetadataService(getter, config.ObjectStoreServicesName)
			if err != nil {
				return nil, errors.Trace(err)
			}

			var leaseManager lease.Manager
			if err := getter.Get(config.LeaseManagerName, &leaseManager); err != nil {
				return nil, errors.Trace(err)
			}

			var objectStoreServicesGetter services.ObjectStoreServicesGetter
			if err := getter.Get(config.ObjectStoreServicesName, &objectStoreServicesGetter); err != nil {
				return nil, errors.Trace(err)
			}

			var s3Client coreobjectstore.Client
			if err := getter.Get(config.S3ClientName, &s3Client); err != nil {
				return nil, errors.Trace(err)
			}

			var apiRemoteCaller apiremotecaller.APIRemoteCallers
			if err := getter.Get(config.APIRemoteCallerName, &apiRemoteCaller); err != nil {
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

			w, err := NewWorker(WorkerConfig{
				TracerGetter:              tracerGetter,
				RootDir:                   dataDir,
				RootBucket:                rootBucketName,
				Clock:                     config.Clock,
				Logger:                    config.Logger,
				NewObjectStoreWorker:      config.NewObjectStoreWorker,
				ObjectStoreType:           controllerConfig.ObjectStoreType(),
				S3Client:                  s3Client,
				APIRemoteCaller:           apiRemoteCaller,
				ControllerMetadataService: metadataService,
				ModelServiceGetter: modelServiceGetter{
					servicesGetter: objectStoreServicesGetter,
				},
				ModelMetadataServiceGetter: modelMetadataServiceGetter{
					servicesGetter: objectStoreServicesGetter,
				},
				ModelClaimGetter: modelClaimGetter{
					manager: leaseManager,
				},
				AllowDraining: AllowDraining(controllerConfig, config.IsBootstrapController(dataDir)),
			})
			return w, errors.Trace(err)
		},
	}
}

func output(in worker.Worker, out any) error {
	w, ok := in.(*objectStoreWorker)
	if !ok {
		return errors.Errorf("expected input of objectStoreWorker, got %T", in)
	}

	switch out := out.(type) {
	case *coreobjectstore.ObjectStoreGetter:
		var target coreobjectstore.ObjectStoreGetter = w
		*out = target

	case *coreobjectstore.ObjectStoreFlusher:
		var target coreobjectstore.ObjectStoreFlusher = w
		*out = target

	default:
		return errors.Errorf("expected output of ObjectStoreGetter, got %T", out)
	}
	return nil
}

func bucketName(config controller.Config) (string, error) {
	name := fmt.Sprintf("juju-%s", config.ControllerUUID())
	if _, err := coreobjectstore.ParseObjectStoreBucketName(name); err != nil {
		return "", errors.Trace(err)
	}
	return name, nil
}

type controllerMetadataService struct {
	factory services.ControllerObjectStoreServices
}

// ObjectStore returns the object store metadata for the controller model.
// This is the global object store.
func (s controllerMetadataService) ObjectStore() coreobjectstore.ObjectStoreMetadata {
	return s.factory.AgentObjectStore()
}

type modelMetadataServiceGetter struct {
	servicesGetter services.ObjectStoreServicesGetter
}

// ForModelUUID returns the MetadataService for the given model UUID.
func (s modelMetadataServiceGetter) ForModelUUID(modelUUID model.UUID) MetadataService {
	return modelMetadataService{factory: s.servicesGetter.ServicesForModel(modelUUID)}
}

type modelMetadataService struct {
	factory services.ObjectStoreServices
}

// ObjectStore returns the object store metadata for the given model UUID
func (s modelMetadataService) ObjectStore() coreobjectstore.ObjectStoreMetadata {
	return s.factory.ObjectStore()
}

type modelServiceGetter struct {
	servicesGetter services.ObjectStoreServicesGetter
}

// ForModelUUID returns the MetadataService for the given model UUID.
func (s modelServiceGetter) ForModelUUID(modelUUID model.UUID) ModelServices {
	return modelService{factory: s.servicesGetter.ServicesForModel(modelUUID)}
}

type modelService struct {
	factory services.ObjectStoreServices
}

// ModelService returns the object store metadata for the given model UUID
func (s modelService) ModelService() ModelService {
	return s.factory.Model()
}

type modelClaimGetter struct {
	manager lease.Manager
}

// ForModelUUID returns the Locker for the given model UUID.
func (s modelClaimGetter) ForModelUUID(modelUUID model.UUID) (objectstore.Claimer, error) {
	leaseClaimer, err := s.manager.Claimer(lease.ObjectStoreNamespace, modelUUID.String())
	if err != nil {
		return nil, errors.Trace(err)
	}
	leaseRevoker, err := s.manager.Revoker(lease.ObjectStoreNamespace, modelUUID.String())
	if err != nil {
		return nil, errors.Trace(err)
	}

	return claimer{
		claimer: leaseClaimer,
		revoker: leaseRevoker,
	}, nil
}

const (
	// defaultClaimDuration is the default duration for the claim.
	defaultClaimDuration = time.Second * 30
)

// Claimer is the implementation of the objectstore.Claimer interface, which
// wraps the lease complexity.
type claimer struct {
	claimer lease.Claimer
	revoker lease.Revoker
}

// Claim attempts to claim an exclusive lock for the hash. If the claim
// is already taken or fails, then an error is returned.
func (l claimer) Claim(ctx context.Context, hash string) (objectstore.ClaimExtender, error) {
	if err := l.claimer.Claim(hash, coreobjectstore.ObjectStoreLeaseHolderName, defaultClaimDuration*2); err != nil {
		return nil, errors.Trace(err)
	}

	return claimExtender{
		claimer: l.claimer,
		hash:    hash,
	}, nil
}

// Release removes the claim for the given hash.
func (l claimer) Release(ctx context.Context, hash string) error {
	return l.revoker.Revoke(hash, coreobjectstore.ObjectStoreLeaseHolderName)
}

type claimExtender struct {
	claimer lease.Claimer
	hash    string
}

// Extend extends the claim for the given hash. This will also check that the
// claim is still valid. If the claim is no longer held, it will claim it
// again.
func (l claimExtender) Extend(ctx context.Context) error {
	return l.claimer.Claim(l.hash, coreobjectstore.ObjectStoreLeaseHolderName, defaultClaimDuration)
}

// Duration returns the duration of the claim.
func (l claimExtender) Duration() time.Duration {
	return defaultClaimDuration
}

// GetControllerConfigService is a helper function that gets a service from the
// manifold.
func GetControllerConfigService(getter dependency.Getter, name string) (ControllerConfigService, error) {
	return coredependency.GetDependencyByName(getter, name, func(factory services.ControllerObjectStoreServices) ControllerConfigService {
		return factory.ControllerConfig()
	})
}

// GetMetadataService is a helper function that gets a service from the
// manifold.
func GetMetadataService(getter dependency.Getter, name string) (MetadataService, error) {
	return coredependency.GetDependencyByName(getter, name, func(factory services.ControllerObjectStoreServices) MetadataService {
		return controllerMetadataService{
			factory: factory,
		}
	})
}

// AllowDraining returns true if the worker should allow draining. This
// currently is only true for the bootstrap controller.
func AllowDraining(config controller.Config, isBootstrapController bool) bool {
	return config.ObjectStoreType() == coreobjectstore.S3Backend && isBootstrapController
}
