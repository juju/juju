// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelworkermanager

import (
	"context"

	"github.com/juju/errors"
	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/dependency"

	"github.com/juju/juju/controller"
	coredependency "github.com/juju/juju/core/dependency"
	"github.com/juju/juju/core/http"
	"github.com/juju/juju/core/lease"
	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/internal/pki"
	"github.com/juju/juju/internal/services"
	jworker "github.com/juju/juju/internal/worker"
)

// GetProviderServicesGetterFunc returns a ProviderServicesGetter
// from the given dependency.Getter.
type GetProviderServicesGetterFunc func(getter dependency.Getter, name string) (ProviderServicesGetter, error)

// ManifoldConfig holds the information necessary to run a model worker manager
// in a dependency.Engine.
type ManifoldConfig struct {
	// AuthorityName is the name of the pki.Authority dependency.
	AuthorityName string
	// DomainServicesName is used to get the controller domain services
	// dependency.
	DomainServicesName string
	// LeaseManagerName is the name of the lease.Manager dependency.
	LeaseManagerName string
	// ProviderServiceFactoriesName is used to get the provider domain services
	// getter dependency. This exposes a provider domain services for each
	// model upon request.
	ProviderServiceFactoriesName string
	// LogSinkName is the name of the logger.ModelLogger dependency.
	LogSinkName string
	// HTTPClientName is the name of the http.Client dependency.
	HTTPClientName string

	// GetProviderServicesGetter is used to get the provider service
	// factory getter from the dependency engine. This makes testing a lot
	// simpler, as we can expose the interface directly, without the
	// intermediary type.
	GetProviderServicesGetter GetProviderServicesGetterFunc

	// GetControllerConfig is used to get the controller config from the
	// controller service.
	GetControllerConfig GetControllerConfigFunc

	// NewWorker is the function that creates the worker.
	NewWorker func(Config) (worker.Worker, error)
	// NewModelWorker is the function that creates the model worker.
	NewModelWorker NewModelWorkerFunc
	// ModelMetrics is the metrics for the model worker.
	ModelMetrics ModelMetrics
	// Logger is the logger for the worker.
	Logger logger.Logger
}

// Validate validates the manifold configuration.
func (config ManifoldConfig) Validate() error {
	if config.AuthorityName == "" {
		return errors.NotValidf("empty AuthorityName")
	}
	if config.DomainServicesName == "" {
		return errors.NotValidf("empty DomainServicesName")
	}
	if config.LeaseManagerName == "" {
		return errors.NotValidf("empty LeaseManagerName")
	}
	if config.ProviderServiceFactoriesName == "" {
		return errors.NotValidf("empty ProviderServiceFactoriesName")
	}
	if config.LogSinkName == "" {
		return errors.NotValidf("empty LogSinkName")
	}
	if config.HTTPClientName == "" {
		return errors.NotValidf("empty HTTPClientName")
	}
	if config.NewWorker == nil {
		return errors.NotValidf("nil NewWorker")
	}
	if config.NewModelWorker == nil {
		return errors.NotValidf("nil NewModelWorker")
	}
	if config.ModelMetrics == nil {
		return errors.NotValidf("nil ModelMetrics")
	}
	if config.Logger == nil {
		return errors.NotValidf("nil Logger")
	}
	if config.GetProviderServicesGetter == nil {
		return errors.NotValidf("nil GetProviderServicesGetter")
	}
	if config.GetControllerConfig == nil {
		return errors.NotValidf("nil GetControllerConfig")
	}
	return nil
}

// Manifold returns a dependency.Manifold that will run a model worker manager.
func Manifold(config ManifoldConfig) dependency.Manifold {
	return dependency.Manifold{
		Inputs: []string{
			config.AuthorityName,
			config.LogSinkName,
			config.LeaseManagerName,
			config.DomainServicesName,
			config.ProviderServiceFactoriesName,
			config.HTTPClientName,
		},
		Start: config.start,
	}
}

// start is a method on ManifoldConfig because it's more readable than a closure.
func (config ManifoldConfig) start(context context.Context, getter dependency.Getter) (worker.Worker, error) {
	if err := config.Validate(); err != nil {
		return nil, errors.Trace(err)
	}

	var authority pki.Authority
	if err := getter.Get(config.AuthorityName, &authority); err != nil {
		return nil, errors.Trace(err)
	}

	var logSinkGetter logger.ModelLogSinkGetter
	if err := getter.Get(config.LogSinkName, &logSinkGetter); err != nil {
		return nil, errors.Trace(err)
	}

	var domainServicesGetter services.DomainServicesGetter
	if err := getter.Get(config.DomainServicesName, &domainServicesGetter); err != nil {
		return nil, errors.Trace(err)
	}

	var controllerDomainServices services.ControllerDomainServices
	if err := getter.Get(config.DomainServicesName, &controllerDomainServices); err != nil {
		return nil, errors.Trace(err)
	}

	var httpClientGetter http.HTTPClientGetter
	if err := getter.Get(config.HTTPClientName, &httpClientGetter); err != nil {
		return nil, errors.Trace(err)
	}

	var leaseManager lease.Manager
	if err := getter.Get(config.LeaseManagerName, &leaseManager); err != nil {
		return nil, errors.Trace(err)
	}

	providerServicesGetter, err := config.GetProviderServicesGetter(getter, config.ProviderServiceFactoriesName)
	if err != nil {
		return nil, errors.Trace(err)
	}

	w, err := config.NewWorker(Config{
		Authority:              authority,
		Logger:                 config.Logger,
		ModelMetrics:           config.ModelMetrics,
		LogSinkGetter:          logSinkGetter,
		NewModelWorker:         config.NewModelWorker,
		ErrorDelay:             jworker.RestartDelay,
		DomainServicesGetter:   domainServicesGetter,
		LeaseManager:           leaseManager,
		ModelService:           controllerDomainServices.Model(),
		ProviderServicesGetter: providerServicesGetter,
		HTTPClientGetter:       httpClientGetter,
		GetControllerConfig:    config.GetControllerConfig,
	})
	if err != nil {
		return nil, errors.Trace(err)
	}
	return w, nil
}

// GetProviderServicesGetter returns a ProviderServicesGetter from
// the given dependency.Getter.
func GetProviderServicesGetter(getter dependency.Getter, name string) (ProviderServicesGetter, error) {
	return coredependency.GetDependencyByName(getter, name, func(servicesGetter services.ProviderServicesGetter) ProviderServicesGetter {
		return providerServicesGetter{servicesGetter: servicesGetter}
	})
}

// GetControllerConfig returns the controller config from the given service.
func GetControllerConfig(ctx context.Context, services services.DomainServices) (controller.Config, error) {
	controllerConfigService := services.ControllerConfig()
	return controllerConfigService.ControllerConfig(ctx)
}

type providerServicesGetter struct {
	servicesGetter services.ProviderServicesGetter
}

// ServicesForModel returns a ProviderServices for the given model.
func (g providerServicesGetter) ServicesForModel(modelUUID string) ProviderServices {
	return providerServices{factory: g.servicesGetter.ServicesForModel(modelUUID)}
}

type providerServices struct {
	factory services.ProviderServices
}

func (f providerServices) Model() ProviderModelService {
	return f.factory.Model()
}

// Cloud returns the cloud service.
func (f providerServices) Cloud() ProviderCloudService {
	return f.factory.Cloud()
}

// Config returns the cloud service.
func (f providerServices) Config() ProviderConfigService {
	return f.factory.Config()
}

// Credential returns the credential service.
func (f providerServices) Credential() ProviderCredentialService {
	return f.factory.Credential()
}
