// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package providertracker

import (
	"context"

	"github.com/juju/errors"
	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/dependency"

	"github.com/juju/juju/caas"
	coredependency "github.com/juju/juju/core/dependency"
	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/environs"
	environscloudspec "github.com/juju/juju/environs/cloudspec"
	"github.com/juju/juju/internal/storage"
	"github.com/juju/juju/internal/worker/modelworkermanager"
)

// Provider is an interface that represents a provider, this can either be
// a CAAS broker or IAAS provider.
type Provider interface {
	environs.Configer
}

// ProviderConfigGetter is an interface that extends
// environs.EnvironConfigGetter to include the ControllerUUID method.
type ProviderConfigGetter interface {
	environs.EnvironConfigGetter

	// ControllerUUID returns the UUID of the controller.
	ControllerUUID() coremodel.UUID
}

// ProviderFunc is a function that returns a provider, this can either be
// a CAAS broker or IAAS provider.
type ProviderFunc[T Provider] func(ctx context.Context, args environs.OpenParams) (T, error)

// Logger defines the methods used by the pruner worker for logging.
type Logger interface {
	Debugf(string, ...interface{})
	Warningf(string, ...interface{})
}

// GetProviderFunc is a helper function that gets a provider from the manifold.
type GetProviderFunc[T Provider] func(context.Context, ProviderConfigGetter) (T, environscloudspec.CloudSpec, error)

// GetProviderServiceFactoryFunc is a helper function that gets a service from
// the manifold.
type GetProviderServiceFactoryFunc func(dependency.Getter, string) (ServiceFactory, error)

// NewWorkerFunc is a function that creates a new Worker.
type NewWorkerFunc[T Provider] func(ctx context.Context, cfg Config[T]) (worker.Worker, error)

// ManifoldConfig describes the resources used by a Worker.
type ManifoldConfig[T Provider] struct {
	// ProviderServiceFactoryName is the name of the service factory that
	// provides the services required by the provider.
	ProviderServiceFactoryName string
	// NewProvider is a function that returns a provider, this can either be
	// a CAAS broker or IAAS provider.
	NewProvider ProviderFunc[T]
	// NewWorker is a function that creates a new Worker.
	NewWorker NewWorkerFunc[T]
	// GetProvider is a helper function that gets a provider from the manifold.
	// This is generalized to allow for different types of providers.
	GetProvider GetProviderFunc[T]
	// GetProviderServiceFactory is a helper function that gets a service from
	// the dependency engine.
	GetProviderServiceFactory GetProviderServiceFactoryFunc
	// Logger represents the methods used by the worker to log details.
	Logger Logger
}

func (cfg ManifoldConfig[T]) Validate() error {
	if cfg.ProviderServiceFactoryName == "" {
		return errors.NotValidf("empty ProviderServiceFactoryName")
	}
	if cfg.NewWorker == nil {
		return errors.NotValidf("nil NewWorker")
	}
	if cfg.Logger == nil {
		return errors.NotValidf("nil Logger")
	}
	if cfg.GetProvider == nil {
		return errors.NotValidf("nil GetProvider")
	}
	if cfg.GetProviderServiceFactory == nil {
		return errors.NotValidf("nil GetProviderServiceFactory")
	}
	return nil
}

// Manifold returns a Manifold that encapsulates a *Worker and exposes it as
// an environs.Environ resource.
func Manifold[T Provider](config ManifoldConfig[T]) dependency.Manifold {
	return dependency.Manifold{
		Inputs: []string{
			config.ProviderServiceFactoryName,
		},
		Output: manifoldOutput[T],
		Start: func(ctx context.Context, getter dependency.Getter) (worker.Worker, error) {
			serviceFactory, err := config.GetProviderServiceFactory(getter, config.ProviderServiceFactoryName)
			if err != nil {
				return nil, errors.Trace(err)
			}

			w, err := config.NewWorker(ctx, Config[T]{
				CloudService:      serviceFactory.Cloud(),
				ConfigService:     serviceFactory.Config(),
				CredentialService: serviceFactory.Credential(),
				ModelService:      serviceFactory.Model(),
				GetProvider:       config.GetProvider,
				Logger:            config.Logger,
			})
			if err != nil {
				return nil, errors.Trace(err)
			}
			return w, nil
		},
	}
}

// ServiceFactory provides access to the services required by the provider.
type ServiceFactory interface {
	// Model returns the model service.
	Model() ModelService
	// Cloud returns the cloud service.
	Cloud() CloudService
	// Config returns the config service.
	Config() ConfigService
	// Credential returns the credential service.
	Credential() CredentialService
}

// GetControllerConfigService is a helper function that gets a service from the
// manifold.
func GetProviderServiceFactory(getter dependency.Getter, name string) (ServiceFactory, error) {
	return coredependency.GetDependencyByName(getter, name, func(factory modelworkermanager.ProviderServiceFactory) ServiceFactory {
		return serviceFactory{
			factory: factory,
		}
	})
}

// serviceFactory is a simple implementation of ProviderServiceFactory.
type serviceFactory struct {
	factory modelworkermanager.ProviderServiceFactory
}

// Model returns the provider model service.
func (f serviceFactory) Model() ModelService {
	return f.factory.Model()
}

// Cloud returns the provider cloud service.
func (f serviceFactory) Cloud() CloudService {
	return f.factory.Cloud()
}

// Config returns the provider config service.
func (f serviceFactory) Config() ConfigService {
	return f.factory.Config()
}

// Credential returns the provider credential service.
func (f serviceFactory) Credential() CredentialService {
	return f.factory.Credential()
}

// IAASGetProvider creates a new provider from the given args.
func IAASGetProvider(newProvider ProviderFunc[environs.Environ]) func(ctx context.Context, getter ProviderConfigGetter) (environs.Environ, environscloudspec.CloudSpec, error) {
	return func(ctx context.Context, getter ProviderConfigGetter) (environs.Environ, environscloudspec.CloudSpec, error) {
		// We can't use newProvider directly, as type invariance prevents us
		// from using it with the environs.GetEnvironAndCloud function.
		// Just wrap it in a closure to work around this.
		provider, spec, err := environs.GetEnvironAndCloud(ctx, getter, func(ctx context.Context, op environs.OpenParams) (environs.Environ, error) {
			return newProvider(ctx, op)
		})
		if err != nil {
			return nil, environscloudspec.CloudSpec{}, errors.Trace(err)
		}
		return provider, *spec, nil
	}
}

// CAASGetProvider creates a new provider from the given args.
func CAASGetProvider(newProvider ProviderFunc[caas.Broker]) func(ctx context.Context, getter ProviderConfigGetter) (caas.Broker, environscloudspec.CloudSpec, error) {
	return func(ctx context.Context, getter ProviderConfigGetter) (caas.Broker, environscloudspec.CloudSpec, error) {
		cloudSpec, err := getter.CloudSpec(ctx)
		if err != nil {
			return nil, environscloudspec.CloudSpec{}, errors.Annotate(err, "cannot get cloud information")
		}

		cfg, err := getter.ModelConfig(ctx)
		if err != nil {
			return nil, environscloudspec.CloudSpec{}, errors.Trace(err)
		}

		broker, err := newProvider(ctx, environs.OpenParams{
			ControllerUUID: getter.ControllerUUID().String(),
			Cloud:          cloudSpec,
			Config:         cfg,
		})
		if err != nil {
			return nil, environscloudspec.CloudSpec{}, errors.Annotate(err, "cannot create caas broker")
		}
		return broker, cloudSpec, nil
	}
}

func manifoldOutput[T Provider](in worker.Worker, out any) error {
	// In order to switch on the type of the provider, we need to use a type
	// assertion to get the underlying value.
	switch any(new(T)).(type) {
	case *environs.Environ:
		w, ok := in.(*trackerWorker[environs.Environ])
		if !ok {
			return errors.Errorf("expected *trackerWorker, got %T", in)
		}
		return iaasOutput(w, out)

	case *caas.Broker:
		w, ok := in.(*trackerWorker[caas.Broker])
		if !ok {
			return errors.Errorf("expected *trackerWorker, got %T", in)
		}
		return caasOutput(w, out)

	default:
		return errors.Errorf("expected *environs.Environ or *caas.Broker, got %T", out)
	}
}

// iaasOutput extracts an environs.Environ resource from a *Worker.
func iaasOutput(in *trackerWorker[environs.Environ], out interface{}) error {
	switch result := out.(type) {
	case *environs.Environ:
		*result = in.Provider()
	case *environs.CloudDestroyer:
		*result = in.Provider()
	case *storage.ProviderRegistry:
		*result = in.Provider()
	default:
		return errors.Errorf("expected *environs.Environ, *storage.ProviderRegistry, or *environs.CloudDestroyer, got %T", out)
	}
	return nil
}

// caasOutput extracts a caas.Broker resource from a *Worker.
func caasOutput(in *trackerWorker[caas.Broker], out interface{}) error {
	switch result := out.(type) {
	case *caas.Broker:
		*result = in.Provider()
	case *environs.CloudDestroyer:
		*result = in.Provider()
	case *storage.ProviderRegistry:
		*result = in.Provider()
	default:
		return errors.Errorf("expected *caas.Broker, *storage.ProviderRegistry or *environs.CloudDestroyer, got %T", out)
	}
	return nil
}
