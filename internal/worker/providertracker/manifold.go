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
	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/core/providertracker"
	"github.com/juju/juju/environs"
	environscloudspec "github.com/juju/juju/environs/cloudspec"
	"github.com/juju/juju/internal/servicefactory"
	"github.com/juju/juju/internal/storage"
	"github.com/juju/juju/internal/worker/modelworkermanager"
)

// Provider is an interface that represents a provider, this can either be
// a CAAS broker or IAAS provider.
type Provider interface {
	providertracker.Provider
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
	Infof(string, ...interface{})
	Debugf(string, ...interface{})
	Warningf(string, ...interface{})
	Errorf(string, ...interface{})
}

// GetProviderFunc is a helper function that gets a provider from the manifold.
type GetProviderFunc[T Provider] func(context.Context, ProviderConfigGetter) (T, environscloudspec.CloudSpec, error)

// GetProviderServiceFactoryGetterFunc is a helper function that gets a service
// factory getter from the manifold.
type GetProviderServiceFactoryGetterFunc func(dependency.Getter, string) (ServiceFactoryGetter, error)

// NewWorkerFunc is a function that creates a new Worker.
type NewWorkerFunc[T Provider] func(cfg Config[T]) (worker.Worker, error)

// NewTrackerWorkerFunc is a function that creates a new TrackerWorker.
type NewTrackerWorkerFunc[T Provider] func(ctx context.Context, cfg TrackerConfig[T]) (worker.Worker, error)

// ManifoldConfig describes the resources used by a Worker.
type ManifoldConfig[T Provider] struct {
	// ProviderServiceFactoriesName is the name of the service factory getter
	// that provides the services required by the provider.
	ProviderServiceFactoriesName string
	// NewProvider is a function that returns a provider, this can either be
	// a CAAS broker or IAAS provider.
	NewProvider ProviderFunc[T]
	// NewWorker is a function that creates a new Worker.
	NewWorker NewWorkerFunc[T]
	// NewTrackerWorker is a function that creates a new TrackerWorker.
	NewTrackerWorker NewTrackerWorkerFunc[T]
	// GetProvider is a helper function that gets a provider from the manifold.
	// This is generalized to allow for different types of providers.
	GetProvider GetProviderFunc[T]
	// GetProviderServiceFactoryGetter is a helper function that gets a service
	// factory getter from the dependency engine.
	GetProviderServiceFactoryGetter GetProviderServiceFactoryGetterFunc
	// Logger represents the methods used by the worker to log details.
	Logger Logger
	// Clock is used by the runner.
	Clock clock.Clock
}

func (cfg ManifoldConfig[T]) Validate() error {
	if cfg.ProviderServiceFactoriesName == "" {
		return errors.NotValidf("empty ProviderServiceFactoriesName")
	}
	if cfg.NewProvider == nil {
		return errors.NotValidf("nil NewProvider")
	}
	if cfg.NewWorker == nil {
		return errors.NotValidf("nil NewWorker")
	}
	if cfg.NewTrackerWorker == nil {
		return errors.NotValidf("nil NewTrackerWorker")
	}
	if cfg.GetProvider == nil {
		return errors.NotValidf("nil GetProvider")
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
func SingularTrackerManifold[T Provider](modelTag names.ModelTag, config ManifoldConfig[T]) dependency.Manifold {
	return manifold[T](SingularType(modelTag.Id()), config)
}

// MultiTrackerManifold creates a new manifold that encapsulates a singular provider
// tracker. Only one tracker is allowed to exist at a time.
func MultiTrackerManifold[T Provider](config ManifoldConfig[T]) dependency.Manifold {
	return manifold[T](MultiType(), config)
}

// manifold returns a Manifold that encapsulates a *Worker and exposes it as
// an environs.Environ resource.
func manifold[T Provider](trackerType TrackerType, config ManifoldConfig[T]) dependency.Manifold {
	return dependency.Manifold{
		Inputs: []string{
			config.ProviderServiceFactoriesName,
		},
		Output: manifoldOutput[T],
		Start: func(ctx context.Context, getter dependency.Getter) (worker.Worker, error) {
			serviceFactoryGetter, err := config.GetProviderServiceFactoryGetter(getter, config.ProviderServiceFactoriesName)
			if err != nil {
				return nil, errors.Trace(err)
			}

			w, err := config.NewWorker(Config[T]{
				TrackerType:          trackerType,
				ServiceFactoryGetter: serviceFactoryGetter,
				GetProvider:          config.GetProvider,
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
		w, ok := in.(*providerWorker[environs.Environ])
		if !ok {
			return errors.Errorf("expected *providerWorker, got %T", in)
		}
		return iaasOutput(w, out)

	case *caas.Broker:
		w, ok := in.(*providerWorker[caas.Broker])
		if !ok {
			return errors.Errorf("expected *providerWorker, got %T", in)
		}
		return caasOutput(w, out)

	default:
		return errors.Errorf("expected *environs.Environ or *caas.Broker, got %T", out)
	}
}

// iaasOutput extracts an environs.Environ resource from a *Worker.
func iaasOutput(in *providerWorker[environs.Environ], out any) error {
	var err error
	switch result := out.(type) {
	case *providertracker.ProviderFactory:
		*result = coerceProviderType[environs.Environ]{Provider: in}
	case *providertracker.GenericProviderFactory[environs.Environ]:
		*result = in
	case *environs.Environ:
		*result, err = in.Provider()
	case *environs.CloudDestroyer:
		*result, err = in.Provider()
	case *storage.ProviderRegistry:
		*result, err = in.Provider()
	default:
		err = errors.Errorf("expected *environs.Environ, *storage.ProviderRegistry, or *environs.CloudDestroyer, got %T", out)
	}
	return errors.Trace(err)
}

// caasOutput extracts a caas.Broker resource from a *Worker.
func caasOutput(in *providerWorker[caas.Broker], out any) error {
	var err error
	switch result := out.(type) {
	case *providertracker.ProviderFactory:
		*result = coerceProviderType[caas.Broker]{Provider: in}
	case *providertracker.GenericProviderFactory[caas.Broker]:
		*result = in
	case *caas.Broker:
		*result, err = in.Provider()
	case *environs.CloudDestroyer:
		*result, err = in.Provider()
	case *storage.ProviderRegistry:
		*result, err = in.Provider()
	default:
		return errors.Errorf("expected *caas.Broker, *storage.ProviderRegistry or *environs.CloudDestroyer, got %T", out)
	}
	return errors.Trace(err)
}

// coerceProviderType forces the type of the provider type to be cast to the
// expected type.
type coerceProviderType[T Provider] struct {
	Provider *providerWorker[T]
}

// ProviderForModel returns the encapsulated provider for a given model
// namespace. It will continue to be updated in the background for as long as
// the Worker continues to run. If the worker is not a singular worker, then an
// error will be returned.
func (w coerceProviderType[T]) ProviderForModel(ctx context.Context, namespace string) (providertracker.Provider, error) {
	provider, err := w.Provider.ProviderForModel(ctx, namespace)
	if err != nil {
		return nil, errors.Trace(err)
	}
	if p, ok := any(provider).(providertracker.Provider); ok {
		return p, nil
	}
	return nil, errors.Errorf("expected providertracker.Provider, got %T", provider)
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
