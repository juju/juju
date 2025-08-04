// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package providertracker

import (
	"context"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/names/v6"
	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/dependency"

	"github.com/juju/juju/caas"
	coredependency "github.com/juju/juju/core/dependency"
	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/providertracker"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/cloudspec"
	"github.com/juju/juju/internal/services"
	"github.com/juju/juju/internal/storage"
	internalworker "github.com/juju/juju/internal/worker"
	"github.com/juju/juju/internal/worker/modelworkermanager"
)

// Provider is an interface that represents a provider, this can either be
// a CAAS broker or IAAS provider.
type Provider = providertracker.Provider

// IAASProviderFunc is a function that returns a IAAS provider.
type IAASProviderFunc func(ctx context.Context, args environs.OpenParams, invalidator environs.CredentialInvalidator) (environs.Environ, error)

// CAASProviderFunc is a function that returns a IAAS provider.
type CAASProviderFunc func(ctx context.Context, args environs.OpenParams, invalidator environs.CredentialInvalidator) (caas.Broker, error)

// GetProviderFunc is a helper function that gets a provider from the manifold.
type GetProviderFunc func(context.Context, environs.EnvironConfigGetter, environs.CredentialInvalidator) (Provider, cloudspec.CloudSpec, error)

// GetProviderServicesGetterFunc is a helper function that gets a service
// factory getter from the manifold.
type GetProviderServicesGetterFunc func(dependency.Getter, string) (DomainServicesGetter, error)

// NewWorkerFunc is a function that creates a new Worker.
type NewWorkerFunc func(cfg Config) (worker.Worker, error)

// NewTrackerWorkerFunc is a function that creates a new TrackerWorker.
type NewTrackerWorkerFunc func(ctx context.Context, cfg TrackerConfig) (worker.Worker, error)

// NewEphemeralProviderFunc is a function that creates a new EphemeralProvider.
type NewEphemeralProviderFunc func(ctx context.Context, cfg EphemeralConfig) (Provider, error)

// ManifoldConfig describes the resources used by a Worker.
type ManifoldConfig struct {
	// ProviderServiceFactoriesName is the name of the domain services getter
	// that provides the services required by the provider.
	ProviderServiceFactoriesName string
	// NewWorker is a function that creates a new Worker.
	NewWorker NewWorkerFunc
	// NewTrackerWorker is a function that creates a new TrackerWorker.
	NewTrackerWorker NewTrackerWorkerFunc
	// NewEphemeralProvider is a function that creates a new ephemeral Provider.
	NewEphemeralProvider NewEphemeralProviderFunc
	// GetIAASProvider is a helper function that gets a IAAS provider from the
	// manifold.
	GetIAASProvider GetProviderFunc
	// GetCAASProvider is a helper function that gets a CAAS provider from the
	// manifold.
	GetCAASProvider GetProviderFunc
	// GetProviderServicesGetter is a helper function that gets a service
	// factory getter from the dependency engine.
	GetProviderServicesGetter GetProviderServicesGetterFunc
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
	if cfg.NewEphemeralProvider == nil {
		return errors.NotValidf("nil NewEphemeralProvider")
	}
	if cfg.GetIAASProvider == nil {
		return errors.NotValidf("nil GetIAASProvider")
	}
	if cfg.GetCAASProvider == nil {
		return errors.NotValidf("nil GetCAASProvider")
	}
	if cfg.GetProviderServicesGetter == nil {
		return errors.NotValidf("nil GetProviderServicesGetter")
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
	m := manifold(SingularType(modelTag.Id()), config)
	m.Filter = internalworker.ShouldWorkerUninstall
	return m
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
			domainServicesGetter, err := config.GetProviderServicesGetter(getter, config.ProviderServiceFactoriesName)
			if err != nil {
				return nil, errors.Trace(err)
			}

			w, err := config.NewWorker(Config{
				TrackerType:          trackerType,
				DomainServicesGetter: domainServicesGetter,
				GetIAASProvider:      config.GetIAASProvider,
				GetCAASProvider:      config.GetCAASProvider,
				NewTrackerWorker:     config.NewTrackerWorker,
				NewEphemeralProvider: config.NewEphemeralProvider,
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
func IAASGetProvider(newProvider IAASProviderFunc) func(ctx context.Context, getter environs.EnvironConfigGetter, invalidator environs.CredentialInvalidator) (Provider, cloudspec.CloudSpec, error) {
	return func(ctx context.Context, getter environs.EnvironConfigGetter, invalidator environs.CredentialInvalidator) (Provider, cloudspec.CloudSpec, error) {
		// We can't use newProvider directly, as type invariance prevents us
		// from using it with the environs.GetEnvironAndCloud function.
		// Just wrap it in a closure to work around this.
		provider, spec, err := environs.GetEnvironAndCloud(ctx, getter, invalidator, func(ctx context.Context, op environs.OpenParams, invalidator environs.CredentialInvalidator) (environs.Environ, error) {
			return newProvider(ctx, op, invalidator)
		})
		if err != nil {
			return nil, cloudspec.CloudSpec{}, errors.Trace(err)
		}
		return provider, *spec, nil
	}
}

// CAASGetProvider creates a new provider from the given args.
func CAASGetProvider(newProvider CAASProviderFunc) func(ctx context.Context, getter environs.EnvironConfigGetter, invalidator environs.CredentialInvalidator) (Provider, cloudspec.CloudSpec, error) {
	return func(ctx context.Context, getter environs.EnvironConfigGetter, invalidator environs.CredentialInvalidator) (Provider, cloudspec.CloudSpec, error) {
		cloudSpec, err := getter.CloudSpec(ctx)
		if err != nil {
			return nil, cloudspec.CloudSpec{}, errors.Annotate(err, "cannot get cloud information")
		}

		cfg, err := getter.ModelConfig(ctx)
		if err != nil {
			return nil, cloudspec.CloudSpec{}, errors.Trace(err)
		}

		controllerUUID, err := getter.ControllerUUID(ctx)
		if err != nil {
			return nil, cloudspec.CloudSpec{}, errors.Trace(err)
		}

		broker, err := newProvider(ctx, environs.OpenParams{
			ControllerUUID: controllerUUID,
			Cloud:          cloudSpec,
			Config:         cfg,
		}, invalidator)
		if err != nil {
			return nil, cloudspec.CloudSpec{}, errors.Annotate(err, "cannot create caas broker")
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

// GetProviderServicesGetter is a helper function that gets a service from the
// manifold.
// This returns a DomainServicesGetter that is constructed directly from the
// domain services.
func GetProviderServicesGetter(getter dependency.Getter, name string) (DomainServicesGetter, error) {
	return coredependency.GetDependencyByName(getter, name, func(factory services.ProviderServicesGetter) DomainServicesGetter {
		return domainServicesGetter{
			factory: factory,
		}
	})
}

// domainServicesGetter is a simple implementation of DomainServicesGetter.
type domainServicesGetter struct {
	factory services.ProviderServicesGetter
}

// ServicesForModel returns a ProviderServices for the given model.
func (g domainServicesGetter) ServicesForModel(modelUUID string) DomainServices {
	return domainServices{
		factory: g.factory.ServicesForModel(modelUUID),
	}
}

// domainServices is a simple implementation of DomainServices.
type domainServices struct {
	factory services.ProviderServices
}

// Model returns the provider model service.
func (f domainServices) Model() ModelService {
	return f.factory.Model()
}

// Cloud returns the provider cloud service.
func (f domainServices) Cloud() CloudService {
	return f.factory.Cloud()
}

// Config returns the provider config service.
func (f domainServices) Config() ConfigService {
	return f.factory.Config()
}

// Credential returns the provider credential service.
func (f domainServices) Credential() CredentialService {
	return f.factory.Credential()
}

// GetModelProviderServicesGetter is a helper function that gets a service
// from the manifold.
// This is a model specific version of GetProviderServicesGetter. As
// the domain services is plucked out of the provider domain services already,
// we have to use a different getter. Ideally we would use the services
// directly, but that's plumbed through the model worker manager config.
// We can't use generics here, as although the types are the same, the nested
// interfaces are not (invariance).
// If the provider domain services returned interfaces, we could just point the
// getter at the domain services directly.
func GetModelProviderServicesGetter(getter dependency.Getter, name string) (DomainServicesGetter, error) {
	return coredependency.GetDependencyByName(getter, name, func(factory modelworkermanager.ProviderServicesGetter) DomainServicesGetter {
		return modelDomainServicesGetter{
			factory: factory,
		}
	})
}

// modelDomainServicesGetter is a simple implementation of DomainServicesGetter.
type modelDomainServicesGetter struct {
	factory modelworkermanager.ProviderServicesGetter
}

// ServicesForModel returns a ProviderServices for the given model.
func (g modelDomainServicesGetter) ServicesForModel(modelUUID string) DomainServices {
	return modelDomainServices{
		factory: g.factory.ServicesForModel(modelUUID),
	}
}

// modelDomainServices is a simple implementation of DomainServices.
type modelDomainServices struct {
	factory modelworkermanager.ProviderServices
}

// Model returns the provider model service.
func (f modelDomainServices) Model() ModelService {
	return f.factory.Model()
}

// Cloud returns the provider cloud service.
func (f modelDomainServices) Cloud() CloudService {
	return f.factory.Cloud()
}

// Config returns the provider config service.
func (f modelDomainServices) Config() ConfigService {
	return f.factory.Config()
}

// Credential returns the provider credential service.
func (f modelDomainServices) Credential() CredentialService {
	return f.factory.Credential()
}
