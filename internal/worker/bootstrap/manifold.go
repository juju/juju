// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package bootstrap

import (
	"context"
	"net/http"

	"github.com/juju/clock"
	jujuerrors "github.com/juju/errors"
	"github.com/juju/worker/v5"
	"github.com/juju/worker/v5/dependency"

	"github.com/juju/juju/core/flags"
	corehttp "github.com/juju/juju/core/http"
	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/providertracker"
	corestatus "github.com/juju/juju/core/status"
	"github.com/juju/juju/internal/bootstrap"
	"github.com/juju/juju/internal/cloudconfig/instancecfg"
	"github.com/juju/juju/internal/errors"
	"github.com/juju/juju/internal/services"
	"github.com/juju/juju/internal/statushistory"
	"github.com/juju/juju/internal/worker/gate"
)

// FlagService is the interface that is used to set the value of a
// flag.
type FlagService interface {
	GetFlag(context.Context, string) (bool, error)
	SetFlag(context.Context, string, bool, string) error
}

// ControllerCharmDeployerFunc is the function that is used to upload the
// controller charm.
type ControllerCharmDeployerFunc func(context.Context, ControllerCharmDeployerConfig) (bootstrap.ControllerCharmDeployer, error)

// PopulateControllerCharmFunc is the function that is used to populate the
// controller charm.
type PopulateControllerCharmFunc func(context.Context, bootstrap.ControllerCharmDeployer) error

// BootstrapAddressFinderGetter is the function that is used to get the
// bootstrap address finder.
type BootstrapAddressFinderGetter func(providerFactory providertracker.ProviderFactory, namespace string) BootstrapAddressFinderFunc

// AgentFinalizerFunc is the function that is used to finalise the agent
// during bootstrap.
type AgentFinalizerFunc func(context.Context, AgentPasswordService, MachineService, instancecfg.StateInitializationParams, string) error

// RemoveBootstrapSSHKeysFunc removes the bootstrap-only SSH keys from the
// machine.
type RemoveBootstrapSSHKeysFunc func([]string) error

// ControllerUnitPasswordFunc is the function that is used to get the
// controller unit password.
type ControllerUnitPasswordFunc func() string

// ControllerApplicationPasswordFunc gets the controller application's unit
// introduction password.
type ControllerApplicationPasswordFunc func() string

// RequiresBootstrapFunc is the function that is used to check if the bootstrap
// process has completed.
type RequiresBootstrapFunc func(context.Context, FlagService) (bool, error)

// HTTPClient is the interface that is used to make HTTP requests.
type HTTPClient interface {
	Do(*http.Request) (*http.Response, error)
}

// StatusHistory records the status of a juju entity to display as its
// status history when requested.
type StatusHistory interface {
	// RecordStatus records the given status information.
	// If the status data cannot be marshalled, it will not be recorded, instead
	// the error will be logged under the data_error key.
	RecordStatus(context.Context, statushistory.Namespace, corestatus.StatusInfo) error
}

// ManifoldConfig defines the configuration for the bootstrap manifold.
type ManifoldConfig struct {
	BootstrapGateName   string
	DomainServicesName  string
	HTTPClientName      string
	ProviderFactoryName string
	// DataDir is the local agent data directory used to read bootstrap params
	// and seed artefacts during bootstrap.
	DataDir string
	// APIPort is the controller API port written into the initial API host-port
	// records once bootstrap completes.
	APIPort int
	// AgentPassword is the bootstrap agent password used by the finaliser to
	// seed the initial machine or controller-node password in state.
	AgentPassword string

	AgentBinaryUploader           AgentBinaryBootstrapFunc
	ControllerCharmDeployer       ControllerCharmDeployerFunc
	ControllerApplicationPassword ControllerApplicationPasswordFunc
	ControllerUnitPassword        ControllerUnitPasswordFunc
	RequiresBootstrap             RequiresBootstrapFunc
	PopulateControllerCharm       PopulateControllerCharmFunc
	BootstrapAddressFinderGetter  BootstrapAddressFinderGetter
	AgentFinalizer                AgentFinalizerFunc
	RemoveBootstrapSSHKeys        RemoveBootstrapSSHKeysFunc
	StatusHistory                 StatusHistory

	Logger logger.Logger
	Clock  clock.Clock
}

// Validate validates the manifold configuration.
func (cfg ManifoldConfig) Validate() error {
	if cfg.BootstrapGateName == "" {
		return jujuerrors.NotValidf("empty BootstrapGateName")
	}
	if cfg.DomainServicesName == "" {
		return jujuerrors.NotValidf("empty DomainServicesName")
	}
	if cfg.HTTPClientName == "" {
		return jujuerrors.NotValidf("empty HTTPClientName")
	}
	if cfg.ProviderFactoryName == "" {
		return jujuerrors.NotValidf("empty ProviderFactoryName")
	}
	if cfg.DataDir == "" {
		return jujuerrors.NotValidf("empty DataDir")
	}
	if cfg.APIPort == 0 {
		return jujuerrors.NotValidf("missing APIPort")
	}
	if cfg.AgentPassword == "" {
		return jujuerrors.NotValidf("missing AgentPassword")
	}

	if cfg.AgentBinaryUploader == nil {
		return jujuerrors.NotValidf("nil AgentBinaryUploader")
	}
	if cfg.ControllerCharmDeployer == nil {
		return jujuerrors.NotValidf("nil ControllerCharmDeployer")
	}
	if cfg.ControllerApplicationPassword == nil {
		return jujuerrors.NotValidf("nil ControllerApplicationPassword")
	}
	if cfg.ControllerUnitPassword == nil {
		return jujuerrors.NotValidf("nil ControllerUnitPassword")
	}
	if cfg.RequiresBootstrap == nil {
		return jujuerrors.NotValidf("nil RequiresBootstrap")
	}
	if cfg.PopulateControllerCharm == nil {
		return jujuerrors.NotValidf("nil PopulateControllerCharm")
	}
	if cfg.BootstrapAddressFinderGetter == nil {
		return jujuerrors.NotValidf("nil BootstrapAddressFinderGetter")
	}
	if cfg.AgentFinalizer == nil {
		return jujuerrors.NotValidf("nil AgentFinalizer")
	}
	if cfg.RemoveBootstrapSSHKeys == nil {
		return jujuerrors.NotValidf("nil RemoveBootstrapSSHKeys")
	}
	if cfg.StatusHistory == nil {
		return jujuerrors.NotValidf("nil StatusHistory")
	}
	if cfg.Logger == nil {
		return jujuerrors.NotValidf("nil Logger")
	}
	if cfg.Clock == nil {
		return jujuerrors.NotValidf("nil Clock")
	}
	return nil
}

// Manifold returns a dependency manifold that runs the bootstrap worker.
func Manifold(config ManifoldConfig) dependency.Manifold {
	return dependency.Manifold{
		Inputs: []string{
			config.BootstrapGateName,
			config.DomainServicesName,
			config.HTTPClientName,
			config.ProviderFactoryName,
		},
		Start: func(ctx context.Context, getter dependency.Getter) (worker.Worker, error) {
			if err := config.Validate(); err != nil {
				return nil, errors.Capture(err)
			}

			var bootstrapUnlocker gate.Unlocker
			if err := getter.Get(config.BootstrapGateName, &bootstrapUnlocker); err != nil {
				return nil, errors.Capture(err)
			}

			var controllerDomainServices services.ControllerDomainServices
			if err := getter.Get(config.DomainServicesName, &controllerDomainServices); err != nil {
				return nil, errors.Capture(err)
			}

			// If bootstrap has completed, then we don't need to
			// bootstrap. Uninstall the worker, as we don't need it running
			// anymore.
			flagService := controllerDomainServices.Flag()
			if ok, err := config.RequiresBootstrap(ctx, flagService); err != nil {
				return nil, errors.Capture(err)
			} else if !ok {
				bootstrapUnlocker.Unlock()
				return nil, dependency.ErrUninstall
			}

			// Locate the controller unit password.
			unitPassword := config.ControllerUnitPassword()
			applicationPassword := config.ControllerApplicationPassword()

			var providerFactory providertracker.ProviderFactory
			if err := getter.Get(config.ProviderFactoryName, &providerFactory); err != nil {
				return nil, errors.Capture(err)
			}

			controllerModel, err := controllerDomainServices.Model().ControllerModel(ctx)
			if err != nil {
				return nil, errors.Errorf(
					"cannot get controller model when making bootstrap worker: %w",
					err,
				)
			}

			serviceManagerGetter := providertracker.ProviderRunner[ServiceManager](
				providerFactory, controllerModel.UUID.String(),
			)

			var httpClientGetter corehttp.HTTPClientGetter
			if err := getter.Get(config.HTTPClientName, &httpClientGetter); err != nil {
				return nil, errors.Capture(err)
			}

			charmhubHTTPClient, err := httpClientGetter.GetHTTPClient(ctx, corehttp.CharmhubPurpose)
			if err != nil {
				return nil, errors.Capture(err)
			}

			var domainServicesGetter services.DomainServicesGetter
			if err := getter.Get(config.DomainServicesName, &domainServicesGetter); err != nil {
				return nil, errors.Capture(err)
			}
			controllerModelDomainServices, err := domainServicesGetter.ServicesForModel(ctx, controllerModel.UUID)
			if err != nil {
				return nil, errors.Capture(err)
			}

			applicationService := controllerModelDomainServices.Application()

			// Select the operation for fresh bootstrap or restoration here.
			// Keep this choice out of the worker's completion and gate handling.
			operation, err := NewFreshBootstrap(FreshBootstrapConfig{
				ControllerAgentBinaryStore: controllerDomainServices.ControllerAgentBinaryStore(),
				ControllerConfigService:    controllerDomainServices.ControllerConfig(),
				ControllerNodeService:      controllerDomainServices.ControllerNode(),
				UserService:                controllerDomainServices.Access(),
				StorageService:             controllerModelDomainServices.Storage(),
				AgentPasswordService:       controllerModelDomainServices.AgentPassword(),
				ApplicationService:         applicationService,
				ControllerModel:            controllerModel,
				ModelConfigService:         controllerModelDomainServices.Config(),
				ModelInfoService:           controllerModelDomainServices.ModelInfo(),
				MachineService:             controllerModelDomainServices.Machine(),
				KeyManagerService:          controllerModelDomainServices.KeyManager(),
				NetworkService:             controllerModelDomainServices.Network(),
				BakeryConfigService:        controllerDomainServices.Macaroon(),
				DataDir:                    config.DataDir,
				APIPort:                    config.APIPort,
				AgentBinaryUploader:        config.AgentBinaryUploader,
				ControllerCharmDeployer:    config.ControllerCharmDeployer,
				PopulateControllerCharm:    config.PopulateControllerCharm,
				AgentFinalizer:             config.AgentFinalizer,
				RemoveBootstrapSSHKeys:     config.RemoveBootstrapSSHKeys,
				AgentPassword:              config.AgentPassword,
				ApplicationPassword:        applicationPassword,
				CharmhubHTTPClient:         charmhubHTTPClient,
				UnitPassword:               unitPassword,
				ServiceManagerGetter:       serviceManagerGetter,
				BootstrapAddressFinder:     config.BootstrapAddressFinderGetter(providerFactory, controllerModel.UUID.String()),
				Logger:                     config.Logger,
				Clock:                      config.Clock,
			})
			if err != nil {
				return nil, errors.Capture(err)
			}
			w, err := NewWorker(WorkerConfig{
				Operation:           operation,
				FlagService:         flagService,
				BootstrapUnlocker:   bootstrapUnlocker,
				ControllerModelUUID: controllerModel.UUID,
				StatusHistory:       config.StatusHistory,
				Logger:              config.Logger,
				Clock:               config.Clock,
			})
			if err != nil {
				return nil, errors.Capture(err)
			}
			return w, nil
		},
	}
}

// RequiresBootstrap is the function that is used to check if the bootstrap
// process has completed.
func RequiresBootstrap(ctx context.Context, flagService FlagService) (bool, error) {
	bootstrapped, err := flagService.GetFlag(ctx, flags.BootstrapFlag)
	if err != nil {
		return false, errors.Capture(err)
	}
	return !bootstrapped, nil
}
