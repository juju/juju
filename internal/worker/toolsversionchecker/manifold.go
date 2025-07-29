// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package toolsversionchecker

import (
	"context"
	"time"

	"github.com/juju/errors"
	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/dependency"

	"github.com/juju/juju/agent"
	coredependency "github.com/juju/juju/core/dependency"
	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/environs/tools"
	"github.com/juju/juju/internal/services"
)

// ManifoldConfig defines the names of the manifolds on which a Manifold will depend.
type ManifoldConfig struct {
	AgentName          string
	DomainServicesName string
	Logger             logger.Logger

	GetModelUUID      func(context.Context, dependency.Getter, string) (model.UUID, error)
	GetDomainServices func(context.Context, dependency.Getter, string, model.UUID) (domainServices, error)
	NewWorker         func(VersionCheckerParams) worker.Worker
}

func (cfg ManifoldConfig) Validate() error {
	if cfg.AgentName == "" {
		return errors.NotValidf("empty AgentName")
	}
	if cfg.DomainServicesName == "" {
		return errors.NotValidf("empty DomainServicesName")
	}
	if cfg.Logger == nil {
		return errors.NotValidf("nil Logger")
	}
	if cfg.GetModelUUID == nil {
		return errors.NotValidf("nil GetModelUUID")
	}
	if cfg.GetDomainServices == nil {
		return errors.NotValidf("nil GetDomainServices")
	}
	if cfg.NewWorker == nil {
		return errors.NotValidf("nil NewWorker")
	}
	return nil
}

// Manifold returns a dependency manifold that runs a toolsversionchecker worker,
// using the api connection resource named in the supplied config.
func Manifold(cfg ManifoldConfig) dependency.Manifold {
	return dependency.Manifold{
		Inputs: []string{
			cfg.AgentName,
			cfg.DomainServicesName,
		},
		Start: cfg.start,
	}
}

func (cfg ManifoldConfig) start(ctx context.Context, getter dependency.Getter) (worker.Worker, error) {
	if err := cfg.Validate(); err != nil {
		return nil, errors.Trace(err)
	}

	modelUUID, err := cfg.GetModelUUID(ctx, getter, cfg.AgentName)
	if err != nil {
		return nil, errors.Trace(err)
	}

	domainServices, err := cfg.GetDomainServices(ctx, getter, cfg.DomainServicesName, modelUUID)
	if err != nil {
		return nil, errors.Trace(err)
	}

	checkerParams := VersionCheckerParams{
		// 4 times a day seems a decent enough amount of checks.
		checkInterval:  time.Hour * 6,
		logger:         cfg.Logger,
		domainServices: domainServices,
		findTools:      tools.FindTools,
	}
	return cfg.NewWorker(checkerParams), nil
}

type domainServices struct {
	config  ModelConfigService
	agent   ModelAgentService
	machine MachineService
}

// GetModelUUID returns the model UUID for the agent running the manifold
func GetModelUUID(ctx context.Context, getter dependency.Getter, agentName string) (model.UUID, error) {
	return coredependency.GetDependencyByName(getter, agentName, func(a agent.Agent) model.UUID {
		return model.UUID(a.CurrentConfig().Model().Id())
	})
}

// GetModelDomainServices returns the model domain services for the given model UUID
func GetModelDomainServices(ctx context.Context, getter dependency.Getter, domainServicesName string, modelUUID model.UUID) (domainServices, error) {
	domainServicesGetter, err := coredependency.GetDependencyByName(getter, domainServicesName, func(s services.DomainServicesGetter) services.DomainServicesGetter {
		return s
	})
	if err != nil {
		return domainServices{}, errors.Trace(err)
	}

	ds, err := domainServicesGetter.ServicesForModel(ctx, modelUUID)
	if err != nil {
		return domainServices{}, errors.Trace(err)
	}
	return domainServices{
		config:  ds.Config(),
		agent:   ds.Agent(),
		machine: ds.Machine(),
	}, nil
}
