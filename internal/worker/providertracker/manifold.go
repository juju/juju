// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package providertracker

import (
	"context"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/names/v5"
	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/dependency"

	"github.com/juju/juju/caas"
	coredependency "github.com/juju/juju/core/dependency"
	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/providertracker"
	"github.com/juju/juju/environs"
	environscloudspec "github.com/juju/juju/environs/cloudspec"
	"github.com/juju/juju/internal/servicefactory"
	"github.com/juju/juju/internal/storage"
	"github.com/juju/juju/internal/uuid"
	"github.com/juju/juju/internal/worker/modelworkermanager"
)

// Provider is an interface that represents a provider, this can either be
// a CAAS broker or IAAS provider.
type Provider = providertracker.Provider

// ProviderConfigGetter is an interface that extends
// environs.EnvironConfigGetter to include the ControllerUUID method.
type ProviderConfigGetter interface {
	environs.EnvironConfigGetter

	// ControllerUUID returns the UUID of the controller.
	ControllerUUID() uuid.UUID
}

// IAASProviderFunc is a function that returns a IAAS provider.
type IAASProviderFunc func(ctx context.Context, args environs.OpenParams) (environs.Environ, error)

// CAASProviderFunc is a function that returns a IAAS provider.
type CAASProviderFunc func(ctx context.Context, args environs.OpenParams) (caas.Broker, error)

// GetProviderFunc is a helper function that gets a provider from the manifold.
type GetProviderFunc func(context.Context, ProviderConfigGetter) (Provider, environscloudspec.CloudSpec, error)

// GetProviderServiceFactoryGetterFunc is a helper function that gets a service
// factory getter from the manifold.
type GetProviderServiceFactoryGetterFunc func(dependency.Getter, string) (ServiceFactoryGetter, error)

// NewWorkerFunc is a function that creates a new Worker.
type NewWorkerFunc func(cfg Config) (worker.Worker, error)

// NewTrackerWorkerFunc is a function that creates a new TrackerWorker.
type NewTrackerWorkerFunc func(ctx context.Context, cfg TrackerConfig) (worker.Worker, error)

// ManifoldConfig describes the resources used by a Worker.
type ManifoldConfig struct {
	// ProviderServiceFactoriesName is the name of the service factory getter
	// that provides the services required by the provider.
	ProviderServiceFactoriesName string
	// NewWorker is a function that creates a new Worker.
	NewWorker NewWorkerFunc
	// NewTrackerWorker is a function that creates a new TrackerWorker.
	NewTrackerWorker NewTrackerWorkerFunc
	// GetIAASProvider is a helper function that gets a IAAS provider from the
	// manifold.
	GetIAASProvider GetProviderFunc
	// GetCAASProvider is a helper function that gets a CAAS provider from the
	// manifold.
	GetCAASProvider GetProviderFunc
	// GetProviderServiceFactoryGetter is a helper function that gets a service
	// factory getter from the dependency engine.
	GetProviderServiceFactoryGetter GetProviderServiceFactoryGetterFunc
	// Logger represents the methods used by the worker to log details.
	Logger logger.Logger
	// Clock is used by the runner.
	Clock clock.Clock
}

func (cfg ManifoldConfig) Validate() error {
	if cfg.ProviderServiceFactoriesName == "" {
		return errors.NotValidf("empty ProviderServiceFactoriesName")
	}
	if cfg.NewWorker == nil {
		return errors.NotValidf("nil NewWorker")
	}
	if cfg.NewTrackerWorker == nil {
		return errors.NotValidf("nil NewTrackerWorker")
	}
	if cfg.GetIAASProvider == nil {
		return errors.NotValidf("nil GetIAASProvider")
	}
	if cfg.GetCAASProvider == nil {
		return errors.NotValidf("nil GetCAASProvider")
	}
	if cfg.GetProviderServiceFactoryGetter == nil {
		return errors.NotValidf("nil GetProviderServiceFactoryGetter")
	}
	if cfg.Logger == nil {
		return errors.NotValidf("nil Logger")
	}
	if cfg.Clock == nil {
		return errors.NotValidf("nil Clock")
	}
	return nil
}

// SingularTrackerManifold creates a new manifold that encapsulates a singular provider
// tracker. Only one tracker is allowed to exist at a time.
func SingularTrackerManifold(modelTag names.ModelTag, config ManifoldConfig) dependency.Manifold {
	return manifold(SingularType(modelTag.Id()), config)
}

// MultiTrackerManifold creates a new manifold that encapsulates a singular provider
// tracker. Only one tracker is allowed to exist at a time.
func MultiTrackerManifold(config ManifoldConfig) dependency.Manifold {
	return manifold(MultiType(), config)
}

// manifold returns a Manifold that encapsulates a *Worker and exposes it as
// an environs.Environ resource.
func manifold(trackerType TrackerType, config ManifoldConfig) dependency.Manifold {
	return dependency.Manifold{
		Inputs: []string{
			config.ProviderServiceFactoriesName,
		},
		Output: manifoldOutput,
		Start: func(ctx context.Context, getter dependency.Getter) (worker.Worker, error) {
			serviceFactoryGetter, err := config.GetProviderServiceFactoryGetter(getter, config.ProviderServiceFactoriesName)
			if err != nil {
				return nil, errors.Trace(err)
			}

			w, err := config.NewWorker(Config{
				TrackerType:          trackerType,
				ServiceFactoryGetter: serviceFactoryGetter,
				GetIAASProvider:      config.GetIAASProvider,
				GetCAASProvider:      config.GetCAASProvider,
				NewTrackerWorker:     config.NewTrackerWorker,
				Logger:               config.Logger,
				Clock:                config.Clock,
			})
			if err != nil {
				return nil, errors.Trace(err)
			}
			return w, nil
		},
	}
}

// IAASGetProvider creates a new provider from the given args.
func IAASGetProvider(newProvider IAASProviderFunc) func(ctx context.Context, getter ProviderConfigGetter) (Provider, environscloudspec.CloudSpec, error) {
	return func(ctx context.Context, getter ProviderConfigGetter) (Provider, environscloudspec.CloudSpec, error) {
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
func CAASGetProvider(newProvider CAASProviderFunc) func(ctx context.Context, getter ProviderConfigGetter) (Provider, environscloudspec.CloudSpec, error) {
	return func(ctx context.Context, getter ProviderConfigGetter) (Provider, environscloudspec.CloudSpec, error) {
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

func manifoldOutput(in worker.Worker, out any) error {
	w, ok := in.(*providerWorker)
	if !ok {
		return errors.Errorf("expected *providerWorker, got %T", in)
	}

	var err error
	switch result := out.(type) {
	case *providertracker.ProviderFactory:
		*result = w
	case *environs.Environ:
		p, err := w.Provider()
		if err != nil {
			return errors.Trace(err)
		}
		environ, ok := p.(environs.Environ)
		if !ok {
			return errors.Errorf("expected *environs.Environ, got %T", p)
		}
		*result = environ
	case *caas.Broker:
		p, err := w.Provider()
		if err != nil {
			return errors.Trace(err)
		}
		broker, ok := p.(caas.Broker)
		if !ok {
			return errors.Errorf("expected *caas.Broker, got %T", p)
		}
		*result = broker
	case *environs.CloudDestroyer:
		*result, err = w.Provider()
	case *storage.ProviderRegistry:
		*result, err = w.Provider()
	default:
		err = errors.NotValidf("*environs.Environ, *caas.Broker, *storage.ProviderRegistry, or *environs.CloudDestroyer: %T", out)
	}
	return errors.Trace(err)
}

// GetProviderServiceFactoryGetter is a helper function that gets a service from the
// manifold.
// This returns a ServiceFactoryGetter that is constructed directly from the
// service factory.
func GetProviderServiceFactoryGetter(getter dependency.Getter, name string) (ServiceFactoryGetter, error) {
	return coredependency.GetDependencyByName(getter, name, func(factory servicefactory.ProviderServiceFactoryGetter) ServiceFactoryGetter {
		return serviceFactoryGetter{
			factory: factory,
		}
	})
}

// serviceFactoryGetter is a simple implementation of ServiceFactoryGetter.
type serviceFactoryGetter struct {
	factory servicefactory.ProviderServiceFactoryGetter
}

// FactoryForModel returns a ProviderServiceFactory for the given model.
func (g serviceFactoryGetter) FactoryForModel(modelUUID string) ServiceFactory {
	return serviceFactory{
		factory: g.factory.FactoryForModel(modelUUID),
	}
}

// serviceFactory is a simple implementation of ServiceFactory.
type serviceFactory struct {
	factory servicefactory.ProviderServiceFactory
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

// GetModelProviderServiceFactoryGetter is a helper function that gets a service
// from the manifold.
// This is a model specific version of GetProviderServiceFactoryGetter. As
// the service factory is plucked out of the provider service factory already,
// we have to use a different getter. Ideally we would use the servicefactory
// directly, but that's plumbed through the model worker manager config.
// We can't use generics here, as although the types are the same, the nested
// interfaces are not (invariance).
// If the provider service factory returned interfaces, we could just point the
// getter at the service factory directly.
func GetModelProviderServiceFactoryGetter(getter dependency.Getter, name string) (ServiceFactoryGetter, error) {
	return coredependency.GetDependencyByName(getter, name, func(factory modelworkermanager.ProviderServiceFactoryGetter) ServiceFactoryGetter {
		return modelServiceFactoryGetter{
			factory: factory,
		}
	})
}

// modelServiceFactoryGetter is a simple implementation of ServiceFactoryGetter.
type modelServiceFactoryGetter struct {
	factory modelworkermanager.ProviderServiceFactoryGetter
}

// FactoryForModel returns a ProviderServiceFactory for the given model.
func (g modelServiceFactoryGetter) FactoryForModel(modelUUID string) ServiceFactory {
	return modelServiceFactory{
		factory: g.factory.FactoryForModel(modelUUID),
	}
}

// modelServiceFactory is a simple implementation of ServiceFactory.
type modelServiceFactory struct {
	factory modelworkermanager.ProviderServiceFactory
}

// Model returns the provider model service.
func (f modelServiceFactory) Model() ModelService {
	return f.factory.Model()
}

// Cloud returns the provider cloud service.
func (f modelServiceFactory) Cloud() CloudService {
	return f.factory.Cloud()
}

// Config returns the provider config service.
func (f modelServiceFactory) Config() ConfigService {
	return f.factory.Config()
}

// Credential returns the provider credential service.
func (f modelServiceFactory) Credential() CredentialService {
	return f.factory.Credential()
}
