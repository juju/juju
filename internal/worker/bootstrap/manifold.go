// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package bootstrap

import (
	"context"
	"io"
	"net/http"

	"github.com/juju/errors"
	"github.com/juju/worker/v3"
	"github.com/juju/worker/v3/dependency"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/core/flags"
	"github.com/juju/juju/core/objectstore"
	"github.com/juju/juju/internal/bootstrap"
	"github.com/juju/juju/internal/servicefactory"
	"github.com/juju/juju/internal/worker/common"
	"github.com/juju/juju/internal/worker/gate"
	workerobjectstore "github.com/juju/juju/internal/worker/objectstore"
	"github.com/juju/juju/internal/worker/state"
	"github.com/juju/juju/state/binarystorage"
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
type AgentBinaryBootstrapFunc func(context.Context, string, BinaryAgentStorageService, objectstore.ObjectStore, Logger) (func(), error)

// ControllerCharmUploaderFunc is the function that is used to upload the
// controller charm.
type ControllerCharmUploaderFunc func(context.Context, string, objectstore.ObjectStore, Logger) error

// HTTPClient is the interface that is used to make HTTP requests.
type HTTPClient interface {
	Do(*http.Request) (*http.Response, error)
}

// ManifoldConfig defines the configuration for the trace manifold.
type ManifoldConfig struct {
	AgentName              string
	StateName              string
	ObjectStoreName        string
	BootstrapGateName      string
	ServiceFactoryName     string
	CharmhubHTTPClientName string

	AgentBinaryUploader     AgentBinaryBootstrapFunc
	ControllerCharmUploader ControllerCharmUploaderFunc
	RequiresBootstrap       func(context.Context, FlagService) (bool, error)
	Logger                  Logger
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
	if cfg.CharmhubHTTPClientName == "" {
		return errors.NotValidf("empty CharmhubHTTPClientName")
	}
	if cfg.Logger == nil {
		return errors.NotValidf("nil Logger")
	}
	if cfg.AgentBinaryUploader == nil {
		return errors.NotValidf("nil AgentBinaryUploader")
	}
	if cfg.ControllerCharmUploader == nil {
		return errors.NotValidf("nil ControllerCharmUploader")
	}
	if cfg.RequiresBootstrap == nil {
		return errors.NotValidf("nil RequiresBootstrap")
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
			config.CharmhubHTTPClientName,
		},
		Start: func(ctx dependency.Context) (worker.Worker, error) {
			if err := config.Validate(); err != nil {
				return nil, errors.Trace(err)
			}

			var bootstrapUnlocker gate.Unlocker
			if err := ctx.Get(config.BootstrapGateName, &bootstrapUnlocker); err != nil {
				return nil, errors.Trace(err)
			}

			var a agent.Agent
			if err := ctx.Get(config.AgentName, &a); err != nil {
				return nil, err
			}

			var controllerServiceFactory servicefactory.ControllerServiceFactory
			if err := ctx.Get(config.ServiceFactoryName, &controllerServiceFactory); err != nil {
				return nil, errors.Trace(err)
			}

			// If the controller application exists, then we don't need to
			// bootstrap. Uninstall the worker, as we don't need it running
			// anymore.
			flagService := controllerServiceFactory.Flag()
			if ok, err := config.RequiresBootstrap(context.TODO(), flagService); err != nil {
				return nil, errors.Trace(err)
			} else if !ok {
				bootstrapUnlocker.Unlock()
				return nil, dependency.ErrUninstall
			}

			var objectStoreGetter workerobjectstore.ObjectStoreGetter
			if err := ctx.Get(config.ObjectStoreName, &objectStoreGetter); err != nil {
				return nil, errors.Trace(err)
			}

			var charmhubHTTPClient HTTPClient
			if err := ctx.Get(config.CharmhubHTTPClientName, &charmhubHTTPClient); err != nil {
				return nil, errors.Trace(err)
			}

			var stTracker state.StateTracker
			if err := ctx.Get(config.StateName, &stTracker); err != nil {
				return nil, errors.Trace(err)
			}

			// Get the state pool after grabbing dependencies so we don't need
			// to remember to call Done on it if they're not running yet.
			statePool, _, err := stTracker.Use()
			if err != nil {
				return nil, errors.Trace(err)
			}

			systemState, err := statePool.SystemState()
			if err != nil {
				_ = stTracker.Done()
				return nil, errors.Trace(err)
			}

			w, err := NewWorker(WorkerConfig{
				Agent:                   a,
				ObjectStoreGetter:       objectStoreGetter,
				ControllerConfigService: controllerServiceFactory.ControllerConfig(),
				FlagService:             flagService,
				State:                   systemState,
				BootstrapUnlocker:       bootstrapUnlocker,
				AgentBinaryUploader:     config.AgentBinaryUploader,
				ControllerCharmUploader: config.ControllerCharmUploader,
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

// CAASAgentBinaryUploader is the function that is used to populate the tools
// for CAAS.
func CAASAgentBinaryUploader(context.Context, string, BinaryAgentStorageService, objectstore.ObjectStore, Logger) (func(), error) {
	// CAAS doesn't need to populate the tools.
	return func() {}, nil
}

// IAASAgentBinaryUploader is the function that is used to populate the tools
// for IAAS.
func IAASAgentBinaryUploader(ctx context.Context, dataDir string, storageService BinaryAgentStorageService, objectStore objectstore.ObjectStore, logger Logger) (func(), error) {
	storage, err := storageService.AgentBinaryStorage(objectStore)
	if err != nil {
		return nil, errors.Trace(err)
	}
	defer storage.Close()

	return bootstrap.PopulateAgentBinary(ctx, dataDir, storage, logger)
}

// RequiresBootstrap is the function that is used to check if the bootstrap
// process has completed.
func RequiresBootstrap(ctx context.Context, flagService FlagService) (bool, error) {
	bootstrapped, err := flagService.GetFlag(ctx, flags.BootstrapFlag)
	if err != nil {
		return false, errors.Trace(err)
	}
	return !bootstrapped, nil
}
