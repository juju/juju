// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package logsinkservices

import (
	"github.com/juju/errors"
	"github.com/juju/worker/v4"
	"gopkg.in/tomb.v2"

	"github.com/juju/juju/core/changestream"
	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/internal/services"
)

// Config is the configuration required for domain services worker.
type Config struct {
	// DBGetter supplies WatchableDB implementations by namespace.
	DBGetter changestream.WatchableDBGetter

	Logger logger.Logger

	NewLogSinkServices LogSinkServicesFn
}

// Validate validates the domain services configuration.
func (config Config) Validate() error {
	if config.DBGetter == nil {
		return errors.NotValidf("nil DBGetter")
	}
	if config.Logger == nil {
		return errors.NotValidf("nil Logger")
	}
	if config.NewLogSinkServices == nil {
		return errors.NotValidf("nil NewLogSinkServices")
	}

	return nil
}

// NewWorker returns a new domain services worker, with the given configuration.
func NewWorker(config Config) (worker.Worker, error) {
	if err := config.Validate(); err != nil {
		return nil, errors.Trace(err)
	}

	w := &servicesWorker{
		services: config.NewLogSinkServices(
			config.DBGetter, config.Logger,
		),
	}
	w.tomb.Go(func() error {
		<-w.tomb.Dying()
		return w.tomb.Err()
	})
	return w, nil
}

// servicesWorker is a worker that holds a reference to a domain services.
// This doesn't actually create them dynamically, it just hands them out
// when asked.
type servicesWorker struct {
	tomb tomb.Tomb

	services services.LogSinkServices
}

// Services returns the log sink domain services getter.
func (w *servicesWorker) Services() services.LogSinkServices {
	return w.services
}

// Kill kills the domain services worker.
func (w *servicesWorker) Kill() {
	w.tomb.Kill(nil)
}

// Wait waits for the domain services worker to stop.
func (w *servicesWorker) Wait() error {
	return w.tomb.Wait()
}
