// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package bootstrap

import (
	"github.com/juju/errors"
	"github.com/juju/worker/v3"
	"github.com/juju/worker/v3/dependency"

	"github.com/juju/juju/core/application"
	"github.com/juju/juju/core/objectstore"
	st "github.com/juju/juju/state"
	"github.com/juju/juju/worker/gate"
	"github.com/juju/juju/worker/state"
)

// Logger represents the logging methods called.
type Logger interface {
	Errorf(message string, args ...any)
	Warningf(message string, args ...any)
	Infof(message string, args ...any)
	Debugf(message string, args ...any)
}

// ManifoldConfig defines the configuration for the trace manifold.
type ManifoldConfig struct {
	StateName         string
	ObjectStoreName   string
	BootstrapGateName string
	Logger            Logger
}

// Validate validates the manifold configuration.
func (cfg ManifoldConfig) Validate() error {
	if cfg.ObjectStoreName == "" {
		return errors.NotValidf("empty ObjectStoreName")
	}
	if cfg.StateName == "" {
		return errors.NotValidf("empty StateName")
	}
	if cfg.BootstrapGateName == "" {
		return errors.NotValidf("empty BootstrapGateName")
	}
	if cfg.Logger == nil {
		return errors.NotValidf("nil Logger")
	}
	return nil
}

// Manifold returns a dependency manifold that runs the trace worker.
func Manifold(config ManifoldConfig) dependency.Manifold {
	return dependency.Manifold{
		Inputs: []string{
			config.StateName,
			config.ObjectStoreName,
			config.BootstrapGateName,
		},
		Start: func(context dependency.Context) (worker.Worker, error) {
			if err := config.Validate(); err != nil {
				return nil, errors.Trace(err)
			}

			var bootstrapUnlocker gate.Unlocker
			if err := context.Get(config.BootstrapGateName, &bootstrapUnlocker); err != nil {
				return nil, errors.Trace(err)
			}

			var stTracker state.StateTracker
			if err := context.Get(config.StateName, &stTracker); err != nil {
				return nil, errors.Trace(err)
			}

			// Get the state pool after grabbing dependencies so we don't need
			// to remember to call Done on it if they're not running yet.
			_, st, err := stTracker.Use()
			if err != nil {
				return nil, errors.Trace(err)
			}
			defer func() {
				_ = stTracker.Done()
			}()

			// If the controller application exists, then we don't need to
			// bootstrap. Uninstall the worker, as we don't need it running
			// anymore.
			if ok, err := requiresBootstrap(&applicationStateService{st: st}); err != nil {
				return nil, errors.Trace(err)
			} else if !ok {
				bootstrapUnlocker.Unlock()
				return nil, dependency.ErrUninstall
			}

			var objectStore objectstore.ObjectStore
			if err := context.Get(config.ObjectStoreName, &objectStore); err != nil {
				return nil, errors.Trace(err)
			}

			w, err := NewWorker(WorkerConfig{
				ObjectStore:       objectStore,
				BootstrapUnlocker: bootstrapUnlocker,
				Logger:            config.Logger,
			})
			if err != nil {
				return nil, errors.Trace(err)
			}
			return w, nil
		},
	}
}

// ApplicationService is the interface that is used to check if an application
// exists.
type ApplicationService interface {
	ApplicationExists(name string) (bool, error)
}

// requiresBootstrap returns true if the controller application does not exist.
func requiresBootstrap(appService ApplicationService) (bool, error) {
	ok, err := appService.ApplicationExists(application.ControllerApplicationName)
	if err != nil {
		return false, errors.Trace(err)
	}
	return !ok, nil
}

type applicationStateService struct {
	st *st.State
}

func (s *applicationStateService) ApplicationExists(name string) (bool, error) {
	_, err := s.st.Application(name)
	if err == nil {
		return true, nil
	}
	if errors.Is(err, errors.NotFound) {
		return false, nil
	}
	return false, errors.Annotatef(err, "application exists")
}
