// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package externalcontrollerupdater

import (
	"context"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/worker/v5"
	"github.com/juju/worker/v5/dependency"

	"github.com/juju/juju/internal/services"
)

// ManifoldConfig holds the dependencies for an external-controller-updater
// worker.
type ManifoldConfig struct {
	// DomainServicesName is the manifold dependency that provides controller
	// domain services, used to access the external controller service for
	// watching, reading, and updating local external controller records.
	DomainServicesName string

	// Clock is used to set the runner's restart delay between peer-controller
	// connection attempts.
	Clock clock.Clock

	// NewExternalControllerWatcherClient returns a client for watching a peer
	// controller's published address changes over a direct API connection.
	NewExternalControllerWatcherClient NewExternalControllerWatcherClientFunc
}

// Validate validates the manifold configuration.
func (cfg ManifoldConfig) Validate() error {
	if cfg.DomainServicesName == "" {
		return errors.NotValidf("empty DomainServicesName")
	}
	if cfg.NewExternalControllerWatcherClient == nil {
		return errors.NotValidf("nil NewExternalControllerWatcherClient")
	}
	if cfg.Clock == nil {
		return errors.NotValidf("nil Clock")
	}
	return nil
}

// Manifold returns a Manifold that runs an externalcontrollerupdater worker.
func Manifold(config ManifoldConfig) dependency.Manifold {
	return dependency.Manifold{
		Inputs: []string{
			config.DomainServicesName,
		},
		Start: func(ctx context.Context, getter dependency.Getter) (worker.Worker, error) {
			if err := config.Validate(); err != nil {
				return nil, errors.Trace(err)
			}
			var controllerDomainServices services.ControllerDomainServices
			if err := getter.Get(config.DomainServicesName, &controllerDomainServices); err != nil {
				return nil, errors.Trace(err)
			}
			w, err := New(
				controllerDomainServices.ExternalController(),
				config.NewExternalControllerWatcherClient,
				config.Clock,
				nil,
			)
			if err != nil {
				return nil, errors.Trace(err)
			}
			return w, nil
		},
	}
}
