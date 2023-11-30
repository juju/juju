// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package bootstrap

import (
	"context"
	"io"

	"github.com/juju/errors"
	"github.com/juju/worker/v3"
	"github.com/juju/worker/v3/dependency"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/controller"
	"github.com/juju/juju/core/application"
	"github.com/juju/juju/core/objectstore"
	"github.com/juju/juju/internal/bootstrap"
	"github.com/juju/juju/internal/servicefactory"
	st "github.com/juju/juju/state"
	"github.com/juju/juju/state/binarystorage"
	"github.com/juju/juju/worker/common"
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

// BinaryAgentStorageService is the interface that is used to get the storage
// for the agent binary.
type BinaryAgentStorageService interface {
	AgentBinaryStorage(objectstore.ObjectStore) (BinaryAgentStorage, error)
}

// BinaryAgentStorage is the interface that is used to store the agent binary.
type BinaryAgentStorage interface {
	// Add adds the agent binary to the storage.
	Add(context.Context, io.Reader, binarystorage.Metadata) error
	// Close closes the storage.
	Close() error
}

// AgentBinaryBootstrapFunc is the function that is used to populate the tools.
type AgentBinaryBootstrapFunc func(context.Context, string, BinaryAgentStorageService, objectstore.ObjectStore, controller.Config, Logger) error

// RequiresBootstrapFunc is the function that is used to check if the controller
// application exists.
type RequiresBootstrapFunc func(appService ApplicationService) (bool, error)

// ManifoldConfig defines the configuration for the trace manifold.
type ManifoldConfig struct {
	AgentName          string
	StateName          string
	ObjectStoreName    string
	BootstrapGateName  string
	ServiceFactoryName string

	Logger            Logger
	AgentBinarySeeder AgentBinaryBootstrapFunc
	RequiresBootstrap RequiresBootstrapFunc
}

// Validate validates the manifold configuration.
func (cfg ManifoldConfig) Validate() error {
	if cfg.AgentName == "" {
		return errors.NotValidf("empty AgentName")
	}
	if cfg.ObjectStoreName == "" {
		return errors.NotValidf("empty ObjectStoreName")
	}
	if cfg.StateName == "" {
		return errors.NotValidf("empty StateName")
	}
	if cfg.BootstrapGateName == "" {
		return errors.NotValidf("empty BootstrapGateName")
	}
	if cfg.ServiceFactoryName == "" {
		return errors.NotValidf("empty ServiceFactoryName")
	}
	if cfg.Logger == nil {
		return errors.NotValidf("nil Logger")
	}
	if cfg.AgentBinarySeeder == nil {
		return errors.NotValidf("nil AgentBinarySeeder")
	}
	return nil
}

// Manifold returns a dependency manifold that runs the trace worker.
func Manifold(config ManifoldConfig) dependency.Manifold {
	return dependency.Manifold{
		Inputs: []string{
			config.AgentName,
			config.StateName,
			config.ObjectStoreName,
			config.BootstrapGateName,
			config.ServiceFactoryName,
		},
		Start: func(context dependency.Context) (worker.Worker, error) {
			if err := config.Validate(); err != nil {
				return nil, errors.Trace(err)
			}

			var a agent.Agent
			if err := context.Get(config.AgentName, &a); err != nil {
				return nil, err
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

			// If the controller application exists, then we don't need to
			// bootstrap. Uninstall the worker, as we don't need it running
			// anymore.
			if ok, err := config.RequiresBootstrap(&applicationStateService{st: st}); err != nil {
				return nil, errors.Trace(err)
			} else if !ok {
				_ = stTracker.Done()
				bootstrapUnlocker.Unlock()
				return nil, dependency.ErrUninstall
			}

			var objectStore objectstore.ObjectStore
			if err := context.Get(config.ObjectStoreName, &objectStore); err != nil {
				_ = stTracker.Done()
				return nil, errors.Trace(err)
			}

			var controllerServiceFactory servicefactory.ControllerServiceFactory
			if err := context.Get(config.ServiceFactoryName, &controllerServiceFactory); err != nil {
				_ = stTracker.Done()
				return nil, errors.Trace(err)
			}

			w, err := NewWorker(WorkerConfig{
				Agent:                   a,
				ObjectStore:             objectStore,
				ControllerConfigService: controllerServiceFactory.ControllerConfig(),
				State:                   st,
				BootstrapUnlocker:       bootstrapUnlocker,
				AgentBinarySeeder:       config.AgentBinarySeeder,
				Logger:                  config.Logger,
			})
			if err != nil {
				_ = stTracker.Done()
				return nil, errors.Trace(err)
			}
			return common.NewCleanupWorker(w, func() {
				// Ensure we clean up the state pool.
				_ = stTracker.Done()
			}), nil
		},
	}
}

// CAASAgentBinarySeeder is the function that is used to populate the tools
// for CAAS.
func CAASAgentBinarySeeder(context.Context, string, BinaryAgentStorageService, objectstore.ObjectStore, controller.Config, Logger) error {
	// CAAS doesn't need to populate the tools.
	return nil
}

// IAASAgentBinarySeeder is the function that is used to populate the tools
// for IAAS.
func IAASAgentBinarySeeder(ctx context.Context, dataDir string, storageService BinaryAgentStorageService, objectStore objectstore.ObjectStore, controllerConfig controller.Config, logger Logger) error {
	storage, err := storageService.AgentBinaryStorage(objectStore)
	if err != nil {
		return errors.Trace(err)
	}
	defer storage.Close()

	return bootstrap.PopulateAgentBinary(ctx, dataDir, storage, controllerConfig, logger)
}

// ApplicationService is the interface that is used to check if an application
// exists.
type ApplicationService interface {
	ApplicationExists(name string) (bool, error)
}

// RequiresBootstrap returns true if the controller application does not exist.
func RequiresBootstrap(appService ApplicationService) (bool, error) {
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
