// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package undertaker

import (
	"context"

	"github.com/juju/clock"
	jujuerrors "github.com/juju/errors"
	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/dependency"

	coredatabase "github.com/juju/juju/core/database"
	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/domain/removal/service"
	"github.com/juju/juju/internal/errors"
	"github.com/juju/juju/internal/services"
)

// ControllerModelService is an interface that defines the methods
// required to interact with the domain services of a controller model.
type ControllerModelService interface {
	// GetDeadModels returns the dead models in the controller.
	GetDeadModels(ctx context.Context) ([]model.UUID, error)
	// WatchModels watches for activated models in the controller.
	// This also watches for changes in the model's state as well.
	WatchModels(ctx context.Context) (watcher.NotifyWatcher, error)
}

// GetControllerModelServiceFunc is a function type that retrieves the model
// services for a controller.
type GetControllerModelServiceFunc func(ctx context.Context, getter dependency.Getter, domainServicesName string) (ControllerModelService, error)

// RemovalService defines the methods required to remove models from the
// database.
type RemovalService interface {
	// DeleteModel removes the model with the given UUID from the database.
	DeleteModel(ctx context.Context) error
}

// RemovalServiceGetter is an interface that defines a method to get a removal
// service.
type RemovalServiceGetter interface {
	// GetRemovalService retrieves the removal service for the given domain
	// services name.
	GetRemovalService(ctx context.Context, uuid model.UUID) (RemovalService, error)
}

// GetDomainServicesGetterFunc is a function type that retrieves the domain
// services getter for a given domain services name.
type GetDomainServicesGetterFunc func(ctx context.Context, getter dependency.Getter, domainServicesName string) (RemovalServiceGetter, error)

// ManifoldConfig holds the information necessary to run a undertaker
// worker in a dependency.Engine.
type ManifoldConfig struct {
	DBAccessorName            string
	DomainServicesName        string
	Logger                    logger.Logger
	Clock                     clock.Clock
	NewWorker                 func(Config) (worker.Worker, error)
	GetControllerModelService GetControllerModelServiceFunc
	GetRemovalServiceGetter   GetDomainServicesGetterFunc
}

// Validate validates the manifold configuration.
func (config ManifoldConfig) Validate() error {
	if config.DBAccessorName == "" {
		return jujuerrors.NotValidf("empty DBAccessorName")
	}
	if config.DomainServicesName == "" {
		return jujuerrors.NotValidf("empty DomainServicesName")
	}
	if config.GetRemovalServiceGetter == nil {
		return jujuerrors.NotValidf("nil GetRemovalServiceGetter")
	}
	if config.NewWorker == nil {
		return jujuerrors.NotValidf("nil NewWorker")
	}
	if config.GetControllerModelService == nil {
		return jujuerrors.NotValidf("nil GetControllerModelService")
	}
	if config.Logger == nil {
		return jujuerrors.NotValidf("nil Logger")
	}
	if config.Clock == nil {
		return jujuerrors.NotValidf("nil Clock")
	}
	return nil
}

// Manifold returns a dependency.Manifold that will run a undertaker
// worker.
func Manifold(config ManifoldConfig) dependency.Manifold {
	return dependency.Manifold{
		Inputs: []string{
			config.DBAccessorName,
			config.DomainServicesName,
		},
		Start: config.start,
	}
}

// start is a method on ManifoldConfig because it's more readable than a closure.
func (config ManifoldConfig) start(ctx context.Context, getter dependency.Getter) (worker.Worker, error) {
	if err := config.Validate(); err != nil {
		return nil, errors.Capture(err)
	}

	var dbDeleter coredatabase.DBDeleter
	if err := getter.Get(config.DBAccessorName, &dbDeleter); err != nil {
		return nil, errors.Capture(err)
	}

	controllerModelService, err := config.GetControllerModelService(ctx, getter, config.DomainServicesName)
	if err != nil {
		return nil, errors.Capture(err)
	}

	removalServiceGetter, err := config.GetRemovalServiceGetter(ctx, getter, config.DomainServicesName)
	if err != nil {
		return nil, errors.Capture(err)
	}

	return config.NewWorker(Config{
		DBDeleter:              dbDeleter,
		ControllerModelService: controllerModelService,
		RemovalServiceGetter:   removalServiceGetter,
		Logger:                 config.Logger,
		Clock:                  config.Clock,
	})
}

// GetControllerModelService retrieves the controller model service
// from the dependency getter using the provided domain services name.
func GetControllerModelService(ctx context.Context, getter dependency.Getter, domainServicesName string) (ControllerModelService, error) {
	var domainServices services.ControllerDomainServices
	if err := getter.Get(domainServicesName, &domainServices); err != nil {
		return nil, errors.Capture(err)
	}
	return domainServices.Model(), nil
}

// GetRemovalServiceGetter retrieves the removal service getter from the
// dependency getter using the provided domain services name.
func GetRemovalServiceGetter(ctx context.Context, getter dependency.Getter, domainServicesName string) (
	RemovalServiceGetter, error) {
	var domainServicesGetter services.DomainServicesGetter
	if err := getter.Get(domainServicesName, &domainServicesGetter); err != nil {
		return nil, errors.Capture(err)
	}
	return &removalServiceGetter{
		getter: domainServicesGetter,
	}, nil
}

type removalServiceGetter struct {
	getter services.DomainServicesGetter
}

func (r *removalServiceGetter) GetRemovalService(ctx context.Context, uuid model.UUID) (RemovalService, error) {
	services, err := r.getter.ServicesForModel(ctx, uuid)
	if err != nil {
		return nil, errors.Capture(err)
	}
	return &removalService{
		removalService: services.Removal(),
		modelUUID:      uuid,
	}, nil
}

type removalService struct {
	removalService *service.WatchableService
	modelUUID      model.UUID
}

func (s *removalService) DeleteModel(ctx context.Context) error {
	if err := s.removalService.DeleteModel(ctx, s.modelUUID); err != nil {
		return errors.Errorf("removal service delete model: %w", err)
	}
	return nil
}
