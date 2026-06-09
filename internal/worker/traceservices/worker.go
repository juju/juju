// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package traceservices

import (
	"github.com/juju/errors"
	"github.com/juju/worker/v5"
	"gopkg.in/tomb.v2"

	"github.com/juju/juju/core/changestream"
	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/internal/services"
)

// Config is the configuration required for the trace services worker.
type Config struct {
	// DBGetter supplies WatchableDB implementations by namespace.
	DBGetter changestream.WatchableDBGetter

	Logger logger.Logger

	NewTraceServices TraceServicesFn
}

// Validate validates the services configuration.
func (config Config) Validate() error {
	if config.DBGetter == nil {
		return errors.NotValidf("nil DBGetter")
	}
	if config.Logger == nil {
		return errors.NotValidf("nil Logger")
	}
	if config.NewTraceServices == nil {
		return errors.NotValidf("nil NewTraceServices")
	}
	return nil
}

// NewWorker returns a new trace services worker, with the given configuration.
func NewWorker(config Config) (worker.Worker, error) {
	if err := config.Validate(); err != nil {
		return nil, errors.Trace(err)
	}

	w := &servicesWorker{
		services: config.NewTraceServices(config.DBGetter, config.Logger),
	}
	w.tomb.Go(func() error {
		<-w.tomb.Dying()
		return w.tomb.Err()
	})
	return w, nil
}

// servicesWorker is a worker that holds a reference to trace services.
type servicesWorker struct {
	tomb tomb.Tomb

	services services.TraceServices
}

// Services returns the trace services.
func (w *servicesWorker) Services() services.TraceServices {
	return w.services
}

// Kill kills the services worker.
func (w *servicesWorker) Kill() {
	w.tomb.Kill(nil)
}

// Wait waits for the services worker to stop.
func (w *servicesWorker) Wait() error {
	return w.tomb.Wait()
}
