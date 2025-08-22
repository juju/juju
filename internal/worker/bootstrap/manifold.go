// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package bootstrap

import (
	"context"
	"fmt"
	"net/http"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/dependency"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/core/flags"
	corehttp "github.com/juju/juju/core/http"
	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/machine"
	"github.com/juju/juju/core/objectstore"
	"github.com/juju/juju/core/providertracker"
	corestatus "github.com/juju/juju/core/status"
	"github.com/juju/juju/internal/bootstrap"
	"github.com/juju/juju/internal/cloudconfig/instancecfg"
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

// ObjectStoreGetter is the interface that is used to get a object store.
type ObjectStoreGetter interface {
	// GetObjectStore returns a object store for the given namespace.
	GetObjectStore(context.Context, string) (objectstore.ObjectStore, error)
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

// AgentFinalizerFunc is the function that is used to finalize the agent
// during bootstrap.
type AgentFinalizerFunc func(context.Context, AgentPasswordService, MachineService, instancecfg.StateInitializationParams, agent.Config) error

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

// StatusHistory records status information into a generalized way.
type StatusHistory interface {
	// RecordStatus records the given status information.
	// If the status data cannot be marshalled, it will not be recorded, instead
	// the error will be logged under the data_error key.
	RecordStatus(context.Context, statushistory.Namespace, corestatus.StatusInfo) error
}

// ManifoldConfig defines the configuration for the trace manifold.
type ManifoldConfig struct {
	AgentName           string
	ObjectStoreName     string
	BootstrapGateName   string
	DomainServicesName  string
	HTTPClientName      string
	ProviderFactoryName string

	AgentBinaryUploader          AgentBinaryBootstrapFunc
	ControllerCharmDeployer      ControllerCharmDeployerFunc
	ControllerUnitPassword       ControllerUnitPasswordFunc
	RequiresBootstrap            RequiresBootstrapFunc
	PopulateControllerCharm      PopulateControllerCharmFunc
	BootstrapAddressFinderGetter BootstrapAddressFinderGetter
	AgentFinalizer               AgentFinalizerFunc
	StatusHistory                StatusHistory

	Logger logger.Logger
	Clock  clock.Clock
}

// Validate validates the manifold configuration.
func (cfg ManifoldConfig) Validate() error {
	if cfg.AgentName == "" {
		return errors.NotValidf("empty AgentName")
	}
	if cfg.ObjectStoreName == "" {
		return errors.NotValidf("empty ObjectStoreName")
	}
	if cfg.BootstrapGateName == "" {
		return errors.NotValidf("empty BootstrapGateName")
	}
	if cfg.DomainServicesName == "" {
		return errors.NotValidf("empty DomainServicesName")
	}
	if cfg.HTTPClientName == "" {
		return errors.NotValidf("empty HTTPClientName")
	}
	if cfg.ProviderFactoryName == "" {
		return errors.NotValidf("empty ProviderFactoryName")
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
	if cfg.BootstrapAddressFinderGetter == nil {
		return errors.NotValidf("nil BootstrapAddressFinderGetter")
	}
	if cfg.AgentFinalizer == nil {
		return errors.NotValidf("nil AgentFinalizer")
	}
	if cfg.StatusHistory == nil {
		return errors.NotValidf("nil StatusHistory")
	}
	if cfg.Logger == nil {
		return errors.NotValidf("nil Logger")
	}
	if cfg.Clock == nil {
		return errors.NotValidf("nil Clock")
	}
	return nil
}

// Manifold returns a dependency manifold that runs the trace worker.
func Manifold(config ManifoldConfig) dependency.Manifold {
	return dependency.Manifold{
		Inputs: []string{
			config.AgentName,
			config.ObjectStoreName,
			config.BootstrapGateName,
			config.DomainServicesName,
			config.HTTPClientName,
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

			var controllerDomainServices services.ControllerDomainServices
			if err := getter.Get(config.DomainServicesName, &controllerDomainServices); err != nil {
				return nil, errors.Trace(err)
			}

			// If the controller application exists, then we don't need to
			// bootstrap. Uninstall the worker, as we don't need it running
			// anymore.
			flagService := controllerDomainServices.Flag()
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

			controllerModel, err := controllerDomainServices.Model().ControllerModel(ctx)
			if err != nil {
				return nil, fmt.Errorf(
					"cannot get controller model when making bootstrap worker: %w",
					err,
				)
			}

			serviceManagerGetter := providertracker.ProviderRunner[ServiceManager](
				providerFactory, controllerModel.UUID.String(),
			)

			var objectStoreGetter objectstore.ObjectStoreGetter
			if err := getter.Get(config.ObjectStoreName, &objectStoreGetter); err != nil {
				return nil, errors.Trace(err)
			}

			var httpClientGetter corehttp.HTTPClientGetter
			if err := getter.Get(config.HTTPClientName, &httpClientGetter); err != nil {
				return nil, errors.Trace(err)
			}

			charmhubHTTPClient, err := httpClientGetter.GetHTTPClient(ctx, corehttp.CharmhubPurpose)
			if err != nil {
				return nil, errors.Trace(err)
			}

			var domainServicesGetter services.DomainServicesGetter
			if err := getter.Get(config.DomainServicesName, &domainServicesGetter); err != nil {
				return nil, errors.Trace(err)
			}
			controllerModelDomainServices, err := domainServicesGetter.ServicesForModel(ctx, controllerModel.UUID)
			if err != nil {
				return nil, errors.Trace(err)
			}

			applicationService := controllerModelDomainServices.Application()

			w, err := NewWorker(WorkerConfig{
				Agent:                      a,
				ObjectStoreGetter:          objectStoreGetter,
				ControllerAgentBinaryStore: controllerDomainServices.ControllerAgentBinaryStore(),
				ControllerConfigService:    controllerDomainServices.ControllerConfig(),
				ControllerNodeService:      controllerDomainServices.ControllerNode(),
				CloudService:               controllerDomainServices.Cloud(),
				UserService:                controllerDomainServices.Access(),
				StorageService:             controllerModelDomainServices.Storage(),
				AgentPasswordService:       controllerModelDomainServices.AgentPassword(),
				ApplicationService:         applicationService,
				ControllerModel:            controllerModel,
				ModelConfigService:         controllerModelDomainServices.Config(),
				MachineService:             controllerModelDomainServices.Machine(),
				KeyManagerService:          controllerModelDomainServices.KeyManager(),
				FlagService:                flagService,
				NetworkService:             controllerModelDomainServices.Network(),
				BakeryConfigService:        controllerDomainServices.Macaroon(),
				BootstrapUnlocker:          bootstrapUnlocker,
				AgentBinaryUploader:        config.AgentBinaryUploader,
				ControllerCharmDeployer:    config.ControllerCharmDeployer,
				PopulateControllerCharm:    config.PopulateControllerCharm,
				AgentFinalizer:             config.AgentFinalizer,
				CharmhubHTTPClient:         charmhubHTTPClient,
				UnitPassword:               unitPassword,
				ServiceManagerGetter:       serviceManagerGetter,
				BootstrapAddressFinder:     config.BootstrapAddressFinderGetter(providerFactory, controllerModel.UUID.String()),
				StatusHistory:              config.StatusHistory,
				Logger:                     config.Logger,
				Clock:                      config.Clock,
			})
			if err != nil {
				return nil, errors.Trace(err)
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
		return false, errors.Trace(err)
	}
	return !bootstrapped, nil
}

// PopulateIAASControllerCharm is the function that is used to populate the
// controller IAAS charm.
func PopulateIAASControllerCharm(ctx context.Context, controllerCharmDeployer bootstrap.ControllerCharmDeployer) error {
	return bootstrap.PopulateIAASControllerCharm(ctx, controllerCharmDeployer)
}

// PopulateCAASControllerCharm is the function that is used to populate the
// controller CAAS charm.
func PopulateCAASControllerCharm(ctx context.Context, controllerCharmDeployer bootstrap.ControllerCharmDeployer) error {
	return bootstrap.PopulateCAASControllerCharm(ctx, controllerCharmDeployer)
}

// IAASAgentFinalizer is the function that is used to finalize the
// IAAS agent during bootstrap.
func IAASAgentFinalizer(
	ctx context.Context,
	agentPasswordService AgentPasswordService,
	machineService MachineService,
	bootstrapParams instancecfg.StateInitializationParams,
	agentConfig agent.Config,
) error {
	// Set machine cloud instance data for the bootstrap machine.
	bootstrapMachineUUID, err := machineService.GetMachineUUID(ctx, machine.Name(agent.BootstrapControllerId))
	if err != nil {
		return errors.Trace(err)
	}

	apiInfo, ok := agentConfig.APIInfo()
	if !ok {
		// If this is missing, we cannot set the machine password or set the
		// machine as provisioned.
		return errors.Errorf("agent config is missing APIInfo for %q", agent.BootstrapControllerId)
	}

	// Set the machine password for the bootstrap controller.
	if err := agentPasswordService.SetMachinePassword(ctx, machine.Name(agent.BootstrapControllerId), apiInfo.Password); err != nil {
		return errors.Trace(err)
	}

	// If this data exists, we consider the machine as provisioned.
	if err := machineService.SetMachineCloudInstance(
		ctx,
		bootstrapMachineUUID,
		bootstrapParams.BootstrapMachineInstanceId,
		bootstrapParams.BootstrapMachineDisplayName,
		agent.BootstrapNonce,
		bootstrapParams.BootstrapMachineHardwareCharacteristics,
	); err != nil {
		return errors.Trace(err)
	}

	return nil
}

// CAASAgentFinalizer is the function that is used to finalize the
// CAAS agent during bootstrap.
func CAASAgentFinalizer(
	ctx context.Context,
	agentPasswordService AgentPasswordService,
	machineService MachineService,
	bootstrapParams instancecfg.StateInitializationParams,
	agentConfig agent.Config,
) error {
	apiInfo, ok := agentConfig.APIInfo()
	if !ok {
		// If this is missing, we cannot set the controller node password.
		return errors.Errorf("agent config is missing APIInfo for %q", agent.BootstrapControllerId)
	}

	// Set the controller node password.
	if err := agentPasswordService.SetControllerNodePassword(ctx, agent.BootstrapControllerId, apiInfo.Password); err != nil {
		return errors.Trace(err)
	}

	return nil
}
