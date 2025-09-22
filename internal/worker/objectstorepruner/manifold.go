// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package objectstorepruner

import (
	"context"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/dependency"

	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/core/objectstore"
	"github.com/juju/juju/internal/services"
)

// ObjectStoreService defines the interface that the pruner needs from
// an object store service.
type ObjectStoreService interface {
	// ListMetadata returns the persistence metadata for all paths.
	ListMetadata(ctx context.Context) ([]objectstore.Metadata, error)
}

// NewWorkerFn is an alias function that allows the creation of
// EventQueueWorker.
type NewWorkerFunc func(WorkerConfig) (worker.Worker, error)

// GetObjectStoreServiceFunc is an alias function that allows the
// retrieval of a ObjectStoreService from a dependency.Getter.
type GetObjectStoreServiceFunc func(dependency.Getter, string) (ObjectStoreService, error)

// GetObjectStoreFunc is an alias function that allows the retrieval of
// a ObjectStore from a dependency.Getter.
type GetObjectStoreFunc func(context.Context, dependency.Getter, string) (objectstore.ObjectStore, error)

// ManifoldConfig defines the names of the manifolds on which a Manifold will
// depend.
type ManifoldConfig struct {
	ObjectStoreServicesName string
	ObjectStoreName         string

	Clock                 clock.Clock
	Logger                logger.Logger
	NewWorker             NewWorkerFunc
	GetObjectStoreService GetObjectStoreServiceFunc
	GetObjectStore        GetObjectStoreFunc
}

func (cfg ManifoldConfig) Validate() error {
	if cfg.ObjectStoreServicesName == "" {
		return errors.NotValidf("empty ObjectStoreServicesName")
	}
	if cfg.ObjectStoreName == "" {
		return errors.NotValidf("empty ObjectStoreName")
	}
	if cfg.Clock == nil {
		return errors.NotValidf("nil Clock")
	}
	if cfg.Logger == nil {
		return errors.NotValidf("nil Logger")
	}
	if cfg.NewWorker == nil {
		return errors.NotValidf("nil NewWorker")
	}
	return nil
}

// Manifold returns a dependency manifold that runs the changestream
// worker, using the resource names defined in the supplied config.
func Manifold(config ManifoldConfig) dependency.Manifold {
	return dependency.Manifold{
		Inputs: []string{
			config.ObjectStoreServicesName,
			config.ObjectStoreName,
		},
		Start: func(ctx context.Context, getter dependency.Getter) (worker.Worker, error) {
			if err := config.Validate(); err != nil {
				return nil, errors.Trace(err)
			}

			objectStoreService, err := config.GetObjectStoreService(getter, config.ObjectStoreServicesName)
			if err != nil {
				return nil, errors.Trace(err)
			}

			objectStore, err := config.GetObjectStore(ctx, getter, config.ObjectStoreName)
			if err != nil {
				return nil, errors.Trace(err)
			}

			cfg := WorkerConfig{
				ObjectStoreService: objectStoreService,
				ObjectStore:        objectStore,
				Clock:              config.Clock,
				Logger:             config.Logger,
			}

			w, err := config.NewWorker(cfg)
			if err != nil {
				return nil, errors.Trace(err)
			}
			return w, nil
		},
	}
}

// NewWorker creates a new objectstore pruner worker.
func NewWorker(cfg WorkerConfig) (worker.Worker, error) {
	return newWorker(cfg)
}

// GetModelObjectStoreService returns a GetObjectStoreServiceFunc that
// retrieves the ObjectStoreService for the specified model UUID.
func GetModelObjectStoreService(modelUUID model.UUID) GetObjectStoreServiceFunc {
	return func(getter dependency.Getter, name string) (ObjectStoreService, error) {
		var objectStoreService services.ObjectStoreServicesGetter
		if err := getter.Get(name, &objectStoreService); err != nil {
			return nil, errors.Trace(err)
		}
		return objectStoreService.ServicesForModel(modelUUID).ObjectStore(), nil
	}
}

// GetControllerObjectStoreService returns a GetObjectStoreServiceFunc that
// retrieves the controller ObjectStoreService.
func GetControllerObjectStoreService(getter dependency.Getter, name string) (ObjectStoreService, error) {
	var objectStoreService services.ControllerObjectStoreServices
	if err := getter.Get(name, &objectStoreService); err != nil {
		return nil, errors.Trace(err)
	}
	return objectStoreService.AgentObjectStore(), nil
}

// GetObjectStore returns a GetObjectStoreFunc that retrieves the ObjectStore
// for the specified namespace.
func GetObjectStore(namespace string) GetObjectStoreFunc {
	return func(ctx context.Context, getter dependency.Getter, name string) (objectstore.ObjectStore, error) {
		var objectStore objectstore.ObjectStoreGetter
		if err := getter.Get(name, &objectStore); err != nil {
			return nil, errors.Trace(err)
		}
		return objectStore.GetObjectStore(ctx, namespace)
	}
}
