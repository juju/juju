// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasapplicationprovisioner

import (
	"context"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/worker/v5"
	"github.com/juju/worker/v5/dependency"

	"github.com/juju/juju/caas"
	"github.com/juju/juju/core/application"
	"github.com/juju/juju/core/logger"
	coreresource "github.com/juju/juju/core/resource"
	"github.com/juju/juju/internal/resource"
	"github.com/juju/juju/internal/resource/charmhub"
	"github.com/juju/juju/internal/services"
)

// GetDomainServicesFunc is a function that returns a services.DomainServices
// from a dependency getter. It exists to allow tests to inject mock services.
type GetDomainServicesFunc func(getter dependency.Getter, name string) (services.DomainServices, error)

// GetDomainServices is a helper function that gets the domain services from
// the manifold.
func GetDomainServices(getter dependency.Getter, name string) (services.DomainServices, error) {
	var domainServices services.DomainServices
	if err := getter.Get(name, &domainServices); err != nil {
		return nil, err
	}
	return domainServices, nil
}

// ManifoldConfig defines a CAAS operator provisioner's dependencies.
type ManifoldConfig struct {
	DomainServicesName string
	BrokerName         string
	ClockName          string
	NewWorker          func(Config) (worker.Worker, error)
	GetDomainServices  GetDomainServicesFunc
	Logger             logger.Logger
}

// Validate is called by start to check for bad configuration.
func (config ManifoldConfig) Validate() error {
	if config.DomainServicesName == "" {
		return errors.NotValidf("empty DomainServicesName")
	}
	if config.BrokerName == "" {
		return errors.NotValidf("empty BrokerName")
	}
	if config.ClockName == "" {
		return errors.NotValidf("empty ClockName")
	}
	if config.NewWorker == nil {
		return errors.NotValidf("nil NewWorker")
	}
	if config.GetDomainServices == nil {
		return errors.NotValidf("nil GetDomainServices")
	}
	if config.Logger == nil {
		return errors.NotValidf("nil Logger")
	}
	return nil
}

func (config ManifoldConfig) start(_ context.Context, getter dependency.Getter) (worker.Worker, error) {
	if err := config.Validate(); err != nil {
		return nil, errors.Trace(err)
	}

	domainServices, err := config.GetDomainServices(getter, config.DomainServicesName)
	if err != nil {
		return nil, errors.Trace(err)
	}

	var clock clock.Clock
	if err := getter.Get(config.ClockName, &clock); err != nil {
		return nil, errors.Trace(err)
	}

	var broker caas.Broker
	if err := getter.Get(config.BrokerName, &broker); err != nil {
		return nil, errors.Trace(err)
	}

	facade := &provisionerFacadeShim{
		appSvc:        domainServices.Application(),
		ctrlConfigSvc: domainServices.ControllerConfig(),
		ctrlNodeSvc:   domainServices.ControllerNode(),
		modelCfgSvc:   domainServices.Config(),
		modelInfoSvc:  domainServices.ModelInfo(),
		removalSvc:    domainServices.Removal(),
	}

	resourceOpenerArgs := resource.ResourceOpenerArgs{
		ResourceService:      domainServices.Resource(),
		ApplicationService:   domainServices.Application(),
		CharmhubClientGetter: charmhub.NewCharmHubOpener(domainServices.Config()),
	}
	var rog ResourceOpenerGetterFunc = func(
		ctx context.Context, appID application.UUID, appName string,
	) (coreresource.Opener, error) {
		return resource.NewResourceOpenerForApplication(ctx, resourceOpenerArgs,
			appName, appID)
	}
	w, err := config.NewWorker(Config{
		ApplicationService:         domainServices.Application(),
		StatusService:              domainServices.Status(),
		AgentPasswordService:       domainServices.AgentPassword(),
		StorageProvisioningService: domainServices.StorageProvisioning(),
		ResourceOpenerGetter:       rog,
		Facade:                     facade,
		Broker:                     broker,
		Clock:                      clock,
		Logger:                     config.Logger,
		NewAppWorker:               NewAppWorker,
	})
	if err != nil {
		return nil, errors.Trace(err)
	}
	return w, nil
}

// Manifold creates a manifold that runs a CAAS operator provisioner. See the
// ManifoldConfig type for discussion about how this can/should evolve.
func Manifold(config ManifoldConfig) dependency.Manifold {
	return dependency.Manifold{
		Inputs: []string{
			config.DomainServicesName,
			config.BrokerName,
			config.ClockName,
		},
		Start: config.start,
	}
}
