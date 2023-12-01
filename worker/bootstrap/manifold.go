// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package bootstrap

import (
	"context"
	"io"
	"os"
	"path/filepath"

	"github.com/juju/errors"
	"github.com/juju/worker/v3"
	"github.com/juju/worker/v3/dependency"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/core/objectstore"
	"github.com/juju/juju/internal/bootstrap"
	"github.com/juju/juju/internal/cloudconfig"
	"github.com/juju/juju/internal/servicefactory"
	"github.com/juju/juju/state/binarystorage"
	"github.com/juju/juju/worker/common"
	"github.com/juju/juju/worker/gate"
	workerobjectstore "github.com/juju/juju/worker/objectstore"
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
type AgentBinaryBootstrapFunc func(context.Context, string, BinaryAgentStorageService, objectstore.ObjectStore, Logger) error

// RequiresBootstrapFunc is the function that is used to check if the bootstrap
// process is required.
type RequiresBootstrapFunc func(agent.Config) (bool, error)

// CompletesBootstrapFunc is the function that is used to complete the bootstrap
// process.
type CompletesBootstrapFunc func(agent.Config) error

// ManifoldConfig defines the configuration for the trace manifold.
type ManifoldConfig struct {
	AgentName          string
	StateName          string
	ObjectStoreName    string
	BootstrapGateName  string
	ServiceFactoryName string

	Logger              Logger
	AgentBinaryUploader AgentBinaryBootstrapFunc
	RequiresBootstrap   RequiresBootstrapFunc
	CompletesBootstrap  CompletesBootstrapFunc
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
	if cfg.AgentBinaryUploader == nil {
		return errors.NotValidf("nil AgentBinaryUploader")
	}
	if cfg.RequiresBootstrap == nil {
		return errors.NotValidf("nil RequiresBootstrap")
	}
	if cfg.CompletesBootstrap == nil {
		return errors.NotValidf("nil CompletesBootstrap")
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

			var bootstrapUnlocker gate.Unlocker
			if err := context.Get(config.BootstrapGateName, &bootstrapUnlocker); err != nil {
				return nil, errors.Trace(err)
			}

			var a agent.Agent
			if err := context.Get(config.AgentName, &a); err != nil {
				return nil, err
			}

			// If the controller application exists, then we don't need to
			// bootstrap. Uninstall the worker, as we don't need it running
			// anymore.
			if ok, err := config.RequiresBootstrap(a.CurrentConfig()); err != nil {
				return nil, errors.Trace(err)
			} else if !ok {
				bootstrapUnlocker.Unlock()
				return nil, dependency.ErrUninstall
			}

			var objectStoreGetter workerobjectstore.ObjectStoreGetter
			if err := context.Get(config.ObjectStoreName, &objectStoreGetter); err != nil {
				return nil, errors.Trace(err)
			}

			var controllerServiceFactory servicefactory.ControllerServiceFactory
			if err := context.Get(config.ServiceFactoryName, &controllerServiceFactory); err != nil {
				return nil, errors.Trace(err)
			}

			var stTracker state.StateTracker
			if err := context.Get(config.StateName, &stTracker); err != nil {
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
				State:                   systemState,
				BootstrapUnlocker:       bootstrapUnlocker,
				AgentBinaryUploader:     config.AgentBinaryUploader,
				CompletesBootstrap:      config.CompletesBootstrap,
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
func CAASAgentBinaryUploader(context.Context, string, BinaryAgentStorageService, objectstore.ObjectStore, Logger) error {
	// CAAS doesn't need to populate the tools.
	return nil
}

// IAASAgentBinaryUploader is the function that is used to populate the tools
// for IAAS.
func IAASAgentBinaryUploader(ctx context.Context, dataDir string, storageService BinaryAgentStorageService, objectStore objectstore.ObjectStore, logger Logger) error {
	storage, err := storageService.AgentBinaryStorage(objectStore)
	if err != nil {
		return errors.Trace(err)
	}
	defer storage.Close()

	return bootstrap.PopulateAgentBinary(ctx, dataDir, storage, logger)
}

// RequiresBootstrap returns true if the bootstrap params file exists.
// It is expected at the end of bootstrap that the file is removed.
func RequiresBootstrap(config agent.Config) (bool, error) {
	_, err := os.Stat(filepath.Join(config.DataDir(), cloudconfig.FileNameBootstrapParams))
	if err != nil && !os.IsNotExist(err) {
		return false, errors.Trace(err)
	}
	return !os.IsNotExist(err), nil
}

// CompletesBootstrap removes the bootstrap params file, completing the
// bootstrap process.
func CompletesBootstrap(config agent.Config) error {
	// Remove the bootstrap params file.
	return os.Remove(filepath.Join(config.DataDir(), cloudconfig.FileNameBootstrapParams))
}
