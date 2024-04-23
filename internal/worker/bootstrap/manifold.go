// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package bootstrap

import (
	"context"
	"net/http"

	"github.com/juju/errors"
	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/dependency"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/caas"
	"github.com/juju/juju/cloud"
	"github.com/juju/juju/controller"
	"github.com/juju/juju/core/credential"
	"github.com/juju/juju/core/flags"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/objectstore"
	"github.com/juju/juju/core/user"
	userservice "github.com/juju/juju/domain/access/service"
	applicationservice "github.com/juju/juju/domain/application/service"
	storageservice "github.com/juju/juju/domain/storage/service"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/envcontext"
	"github.com/juju/juju/internal/bootstrap"
	"github.com/juju/juju/internal/servicefactory"
	"github.com/juju/juju/internal/storage"
	"github.com/juju/juju/internal/worker/common"
	"github.com/juju/juju/internal/worker/gate"
	workerstate "github.com/juju/juju/internal/worker/state"
	"github.com/juju/juju/state/stateenvirons"
)

// LoggerFactory is the interface that is used to create new loggers.
type LoggerFactory interface {
	Child(string) Logger
	ChildWithTags(string, ...string) Logger
	Namespace(string) LoggerFactory
}

// Logger represents the logging methods called.
type Logger interface {
	IsTraceEnabled() bool

	Errorf(string, ...interface{})
	Warningf(string, ...interface{})
	Debugf(string, ...interface{})
	Tracef(string, ...interface{})
}

// ControllerConfigService is the interface that is used to get the
// controller configuration.
type ControllerConfigService interface {
	ControllerConfig(context.Context) (controller.Config, error)
}

// CredentialService is the interface that is used to get the
// cloud credential.
type CredentialService interface {
	CloudCredential(ctx context.Context, key credential.Key) (cloud.Credential, error)
}

// CloudService is the interface that is used to interact with the
// cloud.
type CloudService interface {
	Cloud(context.Context, string) (*cloud.Cloud, error)
}

// StorageService instances save a storage pool to dqlite state.
type StorageService interface {
	CreateStoragePool(ctx context.Context, name string, providerType storage.ProviderType, attrs storageservice.PoolAttrs) error
}

// ApplicationService instances save an application to dqlite state.
type ApplicationService interface {
	CreateApplication(ctx context.Context, name string, params applicationservice.AddApplicationParams, units ...applicationservice.AddUnitParams) error
}

// NetworkService is the interface that is used to interact with the
// network spaces/subnets.
type NetworkService interface {
	// Space returns a space from state that matches the input ID.
	// An error is returned if the space does not exist or if there was a problem
	// accessing its information.
	Space(ctx context.Context, uuid string) (*network.SpaceInfo, error)
	// SpaceByName returns a space from state that matches the input name.
	// An error is returned that satisfied errors.NotFound if the space was not found
	// or an error static any problems fetching the given space.
	SpaceByName(ctx context.Context, name string) (*network.SpaceInfo, error)
	// GetAllSpaces returns all spaces for the model.
	GetAllSpaces(ctx context.Context) (network.SpaceInfos, error)
	// ReloadSpaces loads spaces and subnets from the provider into state.
	ReloadSpaces(ctx context.Context) error
}

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

// UserService is the interface that is used to add a new user to the
// database.
type UserService interface {
	// AddUser will add a new user to the database and return the UUID of the
	// user if successful. If no password is set in the incoming argument,
	// the user will be added with an activation key.
	AddUser(ctx context.Context, arg userservice.AddUserArg) (user.UUID, []byte, error)

	// GetUserByName will return the user with the given name.
	GetUserByName(ctx context.Context, name string) (user.User, error)
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

// NewEnvironFunc is the function that is used to create a new environ.
type NewEnvironFunc func(context.Context, environs.OpenParams) (environs.Environ, error)

// BootstrapAddressesFunc is the function that is used to get the bootstrap
// addresses.
type BootstrapAddressesFunc func(context.Context, environs.Environ, instance.Id) (network.ProviderAddresses, error)

// BootstrapAddressFinderFunc is the function that is used to upload the agent
// binary.
type BootstrapAddressFinderFunc func(context.Context, BootstrapAddressesConfig) (network.ProviderAddresses, error)

// HTTPClient is the interface that is used to make HTTP requests.
type HTTPClient interface {
	Do(*http.Request) (*http.Response, error)
}

// BootstrapAddress attempts to use the provided Environ to get the list of
// instances and its addresses. If the Environ does not implement
// InstanceListener (this is the case of CAAS for the moment), then we
// return the hard-coded 'localhost' address.
func BootstrapAddresses(
	ctx context.Context,
	env environs.Environ,
	bootstrapInstanceID instance.Id,
) (network.ProviderAddresses, error) {
	callCtx := envcontext.WithoutCredentialInvalidator(ctx)
	instanceLister, ok := env.(environs.InstanceLister)
	if !ok {
		return nil, errors.NotSupportedf("bootstrap address not supported on this environ")

	}
	// TODO(nvinuesa): which instanceID to use?
	instances, err := instanceLister.Instances(callCtx, []instance.Id{bootstrapInstanceID})
	if err != nil {
		return nil, errors.Annotate(err, "getting bootstrap instance")
	}
	addrs, err := instances[0].Addresses(callCtx)
	if err != nil {
		return nil, errors.Annotate(err, "getting bootstrap instance addresses")
	}
	return addrs, nil
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
	ControllerCharmDeployer ControllerCharmDeployerFunc
	ControllerUnitPassword  ControllerUnitPasswordFunc
	RequiresBootstrap       RequiresBootstrapFunc
	PopulateControllerCharm PopulateControllerCharmFunc

	BootstrapAddressFinder BootstrapAddressFinderFunc
	NewEnviron             NewEnvironFunc
	BootstrapAddresses     BootstrapAddressesFunc

	LoggerFactory LoggerFactory
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
	if cfg.LoggerFactory == nil {
		return errors.NotValidf("nil LoggerFactory")
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
	if cfg.BootstrapAddressFinder == nil {
		return errors.NotValidf("nil BootstrapAddressFinder")
	}
	if cfg.NewEnviron == nil {
		return errors.NotValidf("nil NewEnviron")
	}
	if cfg.BootstrapAddresses == nil {
		return errors.NotValidf("nil BootstrapAddresses")
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
			modelServiceFactory := serviceFactoryGetter.FactoryForModel(systemState.ModelUUID())

			// TODO (stickupkid): This should be removed once we get rid of
			// the policy and move it into the service factory.
			prechecker, err := stateenvirons.NewInstancePrechecker(systemState, modelServiceFactory.Cloud(), modelServiceFactory.Credential())
			if err != nil {
				_ = stTracker.Done()
				return nil, errors.Trace(err)
			}

			model, err := systemState.Model()
			if err != nil {
				_ = stTracker.Done()
				return nil, errors.Trace(err)
			}
			registry, err := stateenvirons.NewStorageProviderRegistryForModel(
				model, controllerServiceFactory.Cloud(), controllerServiceFactory.Credential(),
				stateenvirons.GetNewEnvironFunc(environs.New),
				stateenvirons.GetNewCAASBrokerFunc(caas.New),
			)
			if err != nil {
				_ = stTracker.Done()
				return nil, errors.Trace(err)
			}

			w, err := NewWorker(WorkerConfig{
				Agent:                   a,
				ObjectStoreGetter:       objectStoreGetter,
				ControllerConfigService: controllerServiceFactory.ControllerConfig(),
				CredentialService:       controllerServiceFactory.Credential(),
				CloudService:            controllerServiceFactory.Cloud(),
				UserService:             controllerServiceFactory.Access(),
				StorageService:          modelServiceFactory.Storage(registry),
				ProviderRegistry:        registry,
				ApplicationService:      modelServiceFactory.Application(registry),
				FlagService:             flagService,
				NetworkService:          modelServiceFactory.Network(),
				SystemState: &stateShim{
					State:      systemState,
					prechecker: prechecker,
				},
				BootstrapUnlocker:       bootstrapUnlocker,
				AgentBinaryUploader:     config.AgentBinaryUploader,
				ControllerCharmDeployer: config.ControllerCharmDeployer,
				PopulateControllerCharm: config.PopulateControllerCharm,
				CharmhubHTTPClient:      charmhubHTTPClient,
				UnitPassword:            unitPassword,
				LoggerFactory:           config.LoggerFactory,
				NewEnviron:              config.NewEnviron,
				BootstrapAddresses:      config.BootstrapAddresses,
				BootstrapAddressFinder:  config.BootstrapAddressFinder,
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
