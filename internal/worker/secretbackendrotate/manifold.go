// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package secretbackendrotate

import (
	"context"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/worker/v5"
	"github.com/juju/worker/v5/dependency"

	"github.com/juju/juju/core/logger"
	corewatcher "github.com/juju/juju/core/watcher"
	"github.com/juju/juju/internal/services"
)

// SecretBackendService provides access to the secret backend operations
// required by the worker.
type SecretBackendService interface {
	// WatchSecretBackendRotationChanges returns a watcher that fires when
	// secret backend token rotation schedules change.
	WatchSecretBackendRotationChanges(ctx context.Context) (corewatcher.SecretBackendRotateWatcher, error)
	// RotateBackendToken rotates the token for the given secret backend.
	RotateBackendToken(ctx context.Context, backendID string) error
}

// GetSecretBackendServiceFunc is a helper function that gets the controller
// secret backend service from the dependency getter.
type GetSecretBackendServiceFunc func(getter dependency.Getter, name string) (SecretBackendService, error)

// ManifoldConfig holds dependencies and configuration for a
// secretbackendrotate worker.
type ManifoldConfig struct {
	Logger                  logger.Logger
	DomainServicesName      string
	GetSecretBackendService GetSecretBackendServiceFunc
	NewWorker               func(Config) (worker.Worker, error)
}

// Validate validates a manifold config.
func (c ManifoldConfig) Validate() error {
	if c.DomainServicesName == "" {
		return errors.NotValidf("missing DomainServicesName")
	}
	if c.Logger == nil {
		return errors.NotValidf("nil Logger")
	}
	if c.GetSecretBackendService == nil {
		return errors.NotValidf("nil GetSecretBackendService")
	}
	if c.NewWorker == nil {
		return errors.NotValidf("nil NewWorker")
	}
	return nil
}

// Manifold returns a dependency.Manifold that runs a secretbackendrotate worker.
func Manifold(config ManifoldConfig) dependency.Manifold {
	return dependency.Manifold{
		Inputs: []string{
			config.DomainServicesName,
		},
		Start: config.start,
	}
}

func (c ManifoldConfig) start(_ context.Context, getter dependency.Getter) (worker.Worker, error) {
	if err := c.Validate(); err != nil {
		return nil, errors.Trace(err)
	}

	svc, err := c.GetSecretBackendService(getter, c.DomainServicesName)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return c.NewWorker(Config{
		SecretBackendManagerFacade: svc,
		Logger:                     c.Logger,
		Clock:                      clock.WallClock,
	})
}

// GetSecretBackendService retrieves the secret backend service from the
// controller domain services via the dependency getter.
func GetSecretBackendService(getter dependency.Getter, name string) (SecretBackendService, error) {
	var controllerServices services.ControllerDomainServices
	if err := getter.Get(name, &controllerServices); err != nil {
		return nil, errors.Trace(err)
	}
	return controllerServices.SecretBackend(), nil
}
