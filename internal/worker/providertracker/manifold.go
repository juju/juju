// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package providertracker

import (
	"context"

	"github.com/juju/errors"
	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/dependency"

	coredependency "github.com/juju/juju/core/dependency"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/internal/storage"
	"github.com/juju/juju/internal/worker/modelworkermanager"
)

// Logger defines the methods used by the pruner worker for logging.
type Logger interface {
	Debugf(string, ...interface{})
	Warningf(string, ...interface{})
}

// GetProviderServiceFactoryFunc is a helper function that gets a service from
// the manifold.
type GetProviderServiceFactoryFunc func(dependency.Getter, string) (ServiceFactory, error)

// NewWorkerFunc is a function that creates a new Worker.
type NewWorkerFunc func(ctx context.Context, cfg Config) (worker.Worker, error)

// ManifoldConfig describes the resources used by a Worker.
type ManifoldConfig struct {
	ProviderServiceFactoryName string
	NewEnviron                 environs.NewEnvironFunc
	NewWorker                  NewWorkerFunc
	Logger                     Logger
	GetProviderServiceFactory  GetProviderServiceFactoryFunc
}

func (cfg ManifoldConfig) Validate() error {
	if cfg.ProviderServiceFactoryName == "" {
		return errors.NotValidf("empty ProviderServiceFactoryName")
	}
	if cfg.NewEnviron == nil {
		return errors.NotValidf("nil NewEnviron")
	}
	if cfg.NewWorker == nil {
		return errors.NotValidf("nil NewWorker")
	}
	if cfg.Logger == nil {
		return errors.NotValidf("nil Logger")
	}
	if cfg.GetProviderServiceFactory == nil {
		return errors.NotValidf("nil GetProviderServiceFactory")
	}
	return nil
}

// Manifold returns a Manifold that encapsulates a *Worker and exposes it as
// an environs.Environ resource.
func Manifold(config ManifoldConfig) dependency.Manifold {
	return dependency.Manifold{
		Inputs: []string{
			config.ProviderServiceFactoryName,
		},
		Output: manifoldOutput,
		Start: func(ctx context.Context, getter dependency.Getter) (worker.Worker, error) {
			serviceFactory, err := config.GetProviderServiceFactory(getter, config.ProviderServiceFactoryName)
			if err != nil {
				return nil, errors.Trace(err)
			}

			w, err := config.NewWorker(ctx, Config{
				CloudService:      serviceFactory.Cloud(),
				ConfigService:     serviceFactory.Config(),
				CredentialService: serviceFactory.Credential(),
				ModelService:      serviceFactory.Model(),
				NewEnviron:        config.NewEnviron,
				Logger:            config.Logger,
			})
			if err != nil {
				return nil, errors.Trace(err)
			}
			return w, nil
		},
	}
}

// manifoldOutput extracts an environs.Environ resource from a *Worker.
func manifoldOutput(in worker.Worker, out interface{}) error {
	w, ok := in.(*trackerWorker)
	if !ok {
		return errors.Errorf("expected *environ.Tracker, got %T", in)
	}
	switch result := out.(type) {
	case *environs.Environ:
		*result = w.Environ()
	case *environs.CloudDestroyer:
		*result = w.Environ()
	case *storage.ProviderRegistry:
		*result = w.Environ()
	default:
		return errors.Errorf("expected *environs.Environ, *storage.ProviderRegistry, or *environs.CloudDestroyer, got %T", out)
	}
	return nil
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
