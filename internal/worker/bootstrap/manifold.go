// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package bootstrap

import (
	"context"
	"fmt"
	"net/http"

	"github.com/juju/errors"
	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/dependency"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/core/flags"
	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/objectstore"
	"github.com/juju/juju/core/providertracker"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/internal/bootstrap"
	"github.com/juju/juju/internal/servicefactory"
	"github.com/juju/juju/internal/storage"
	"github.com/juju/juju/internal/worker/common"
	"github.com/juju/juju/internal/worker/gate"
	workerstate "github.com/juju/juju/internal/worker/state"
)

// FlagService is the interface that is used to set the value of a
// flag.
type FlagService interface {
	GetFlag(context.Context, string) (bool, error)
	SetFlag(context.Context, string, bool, string) error
}

// ObjectStoreGetter is the interface that is used to get a object store.
type ObjectStoreGetter interface {
	// GetObjectStore returns a object store for the given namespace.
	GetObjectStore(context.Context, string) (objectstore.ObjectStore, error)
}

// ControllerCharmDeployerFunc is the function that is used to upload the
// controller charm.
type ControllerCharmDeployerFunc func(ControllerCharmDeployerConfig) (bootstrap.ControllerCharmDeployer, error)

// PopulateControllerCharmFunc is the function that is used to populate the
// controller charm.
type PopulateControllerCharmFunc func(context.Context, bootstrap.ControllerCharmDeployer) error

// ControllerUnitPasswordFunc is the function that is used to get the
// controller unit password.
type ControllerUnitPasswordFunc func(context.Context) (string, error)

// RequiresBootstrapFunc is the function that is used to check if the bootstrap
// process has completed.
type RequiresBootstrapFunc func(context.Context, FlagService) (bool, error)

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
	ProviderFactoryName    string

	AgentBinaryUploader     AgentBinaryBootstrapFunc
	ControllerCharmDeployer ControllerCharmDeployerFunc
	ControllerUnitPassword  ControllerUnitPasswordFunc
	RequiresBootstrap       RequiresBootstrapFunc
	PopulateControllerCharm PopulateControllerCharmFunc

	Logger logger.Logger
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
	if cfg.ProviderFactoryName == "" {
		return errors.NotValidf("empty ProviderFactoryName")
	}
	if cfg.Logger == nil {
		return errors.NotValidf("nil Logger")
	}
	if cfg.AgentBinaryUploader == nil {
		return errors.NotValidf("nil AgentBinaryUploader")
	}
	if cfg.ControllerCharmDeployer == nil {
		return errors.NotValidf("nil ControllerCharmDeployer")
	}
	if cfg.ControllerUnitPassword == nil {
		return errors.NotValidf("nil ControllerUnitPassword")
	}
	if cfg.RequiresBootstrap == nil {
		return errors.NotValidf("nil RequiresBootstrap")
	}
	if cfg.PopulateControllerCharm == nil {
		return errors.NotValidf("nil PopulateControllerCharm")
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
			config.ProviderFactoryName,
		},
		Start: func(ctx context.Context, getter dependency.Getter) (worker.Worker, error) {
			if err := config.Validate(); err != nil {
				return nil, errors.Trace(err)
			}

			var bootstrapUnlocker gate.Unlocker
			if err := getter.Get(config.BootstrapGateName, &bootstrapUnlocker); err != nil {
				return nil, errors.Trace(err)
			}

			var a agent.Agent
			if err := getter.Get(config.AgentName, &a); err != nil {
				return nil, err
			}

			var controllerServiceFactory servicefactory.ControllerServiceFactory
			if err := getter.Get(config.ServiceFactoryName, &controllerServiceFactory); err != nil {
				return nil, errors.Trace(err)
			}

			// If the controller application exists, then we don't need to
			// bootstrap. Uninstall the worker, as we don't need it running
			// anymore.
			flagService := controllerServiceFactory.Flag()
			if ok, err := config.RequiresBootstrap(ctx, flagService); err != nil {
				return nil, errors.Trace(err)
			} else if !ok {
				bootstrapUnlocker.Unlock()
				return nil, dependency.ErrUninstall
			}

			// Locate the controller unit password.
			unitPassword, err := config.ControllerUnitPassword(context.TODO())
			if err != nil {
				return nil, errors.Trace(err)
			}

			var providerFactory providertracker.ProviderFactory
			if err := getter.Get(config.ProviderFactoryName, &providerFactory); err != nil {
				return nil, errors.Trace(err)
			}

			controllerModel, err := controllerServiceFactory.Model().ControllerModel(ctx)
			if err != nil {
				return nil, fmt.Errorf(
					"cannot get controller model when making bootstrap worker: %w",
					err,
				)
			}

			instanceListerProvider := providertracker.ProviderRunner[environs.InstanceLister](
				providerFactory, controllerModel.UUID.String(),
			)
			addressFinder := BootstrapAddressFinder(instanceListerProvider)

			var objectStoreGetter objectstore.ObjectStoreGetter
			if err := getter.Get(config.ObjectStoreName, &objectStoreGetter); err != nil {
				return nil, errors.Trace(err)
			}

			var charmhubHTTPClient HTTPClient
			if err := getter.Get(config.CharmhubHTTPClientName, &charmhubHTTPClient); err != nil {
				return nil, errors.Trace(err)
			}

			var stTracker workerstate.StateTracker
			if err := getter.Get(config.StateName, &stTracker); err != nil {
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

			var serviceFactoryGetter servicefactory.ServiceFactoryGetter
			if err := getter.Get(config.ServiceFactoryName, &serviceFactoryGetter); err != nil {
				_ = stTracker.Done()
				return nil, errors.Trace(err)
			}
			controllerModelServiceFactory := serviceFactoryGetter.FactoryForModel(controllerModel.UUID)

			tracker, err := providerFactory.ProviderForModel(ctx, controllerModel.UUID.String())
			if err != nil {
				_ = stTracker.Done()
				return nil, errors.Trace(err)
			}

			registry := storage.NewChainedProviderRegistry(tracker)

			w, err := NewWorker(WorkerConfig{
				Agent:                   a,
				ObjectStoreGetter:       objectStoreGetter,
				ControllerConfigService: controllerServiceFactory.ControllerConfig(),
				CloudService:            controllerServiceFactory.Cloud(),
				UserService:             controllerServiceFactory.Access(),
				StorageService:          controllerModelServiceFactory.Storage(registry),
				ProviderRegistry:        registry,
				ApplicationService:      controllerModelServiceFactory.Application(registry),
				ControllerModel:         controllerModel,
				ModelConfigService:      controllerModelServiceFactory.Config(),
				KeyManagerService:       controllerModelServiceFactory.KeyManager(),
				FlagService:             flagService,
				NetworkService:          controllerModelServiceFactory.Network(),
				BakeryConfigService:     controllerServiceFactory.Macaroon(),
				SystemState: &stateShim{
					State:      systemState,
					prechecker: tracker,
				},
				BootstrapUnlocker:       bootstrapUnlocker,
				AgentBinaryUploader:     config.AgentBinaryUploader,
				ControllerCharmDeployer: config.ControllerCharmDeployer,
				PopulateControllerCharm: config.PopulateControllerCharm,
				CharmhubHTTPClient:      charmhubHTTPClient,
				UnitPassword:            unitPassword,
				Logger:                  config.Logger,
				BootstrapAddressFinder:  addressFinder,
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

// RequiresBootstrap is the function that is used to check if the bootstrap
// process has completed.
func RequiresBootstrap(ctx context.Context, flagService FlagService) (bool, error) {
	bootstrapped, err := flagService.GetFlag(ctx, flags.BootstrapFlag)
	if err != nil {
		return false, errors.Trace(err)
	}
	return !bootstrapped, nil
}

// PopulateControllerCharm is the function that is used to populate the
// controller charm.
func PopulateControllerCharm(ctx context.Context, controllerCharmDeployer bootstrap.ControllerCharmDeployer) error {
	return bootstrap.PopulateControllerCharm(ctx, controllerCharmDeployer)
}
