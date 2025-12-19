// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package toolsversionupdater

import (
	"context"

	"github.com/juju/clock"
	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/dependency"

	"github.com/juju/juju/agent"
	coredependency "github.com/juju/juju/core/dependency"
	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/model"
	errors "github.com/juju/juju/internal/errors"
	"github.com/juju/juju/internal/services"
)

// GetModelUUIDFunc is the type of function that returns the model UUID for the
// agent running the manifold.
type GetModelUUIDFunc func(context.Context, dependency.Getter, string) (model.UUID, error)

// GetDomainServicesFunc is the type of function that returns the model domain
// services for the given model UUID.
type GetDomainServicesFunc func(context.Context, dependency.Getter, string, model.UUID) (domainServices, error)

// NewWorkerFunc is the type of function that creates a new tools version
// updater worker.
type NewWorkerFunc func(WorkerConfig) worker.Worker

// ManifoldConfig defines the configuration for the tools version updater
// manifold.
type ManifoldConfig struct {
	AgentName          string
	DomainServicesName string

	GetModelUUID      GetModelUUIDFunc
	GetDomainServices GetDomainServicesFunc
	NewWorker         NewWorkerFunc

	Clock  clock.Clock
	Logger logger.Logger
}

// Validate validates the manifold configuration.
func (c *ManifoldConfig) Validate() error {
	if c.AgentName == "" {
		return errors.Errorf("empty AgentName").Add(coreerrors.NotValid)
	}
	if c.DomainServicesName == "" {
		return errors.Errorf("empty DomainServicesName").Add(coreerrors.NotValid)
	}
	if c.Clock == nil {
		return errors.Errorf("nil Clock").Add(coreerrors.NotValid)
	}
	if c.Logger == nil {
		return errors.Errorf("nil Logger").Add(coreerrors.NotValid)
	}
	return nil
}

func Manifold(cfg ManifoldConfig) dependency.Manifold {
	return dependency.Manifold{
		Inputs: []string{
			cfg.DomainServicesName,
		},
		Start: func(ctx context.Context, getter dependency.Getter) (worker.Worker, error) {
			if err := cfg.Validate(); err != nil {
				return nil, errors.Capture(err)
			}

			modelUUID, err := cfg.GetModelUUID(ctx, getter, cfg.AgentName)
			if err != nil {
				return nil, errors.Capture(err)
			}

			domainServices, err := cfg.GetDomainServices(ctx, getter, cfg.DomainServicesName, modelUUID)
			if err != nil {
				return nil, errors.Capture(err)
			}

			return cfg.NewWorker(WorkerConfig{
				DomainServices: domainServices,
				Clock:          cfg.Clock,
				Logger:         cfg.Logger,
			}), nil
		},
	}
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
		return domainServices{}, errors.Capture(err)
	}

	ds, err := domainServicesGetter.ServicesForModel(ctx, modelUUID)
	if err != nil {
		return domainServices{}, errors.Capture(err)
	}
	return domainServices{
		config:  ds.Config(),
		agent:   ds.Agent(),
		machine: ds.Machine(),
	}, nil
}

type domainServices struct {
	config  ModelConfigService
	agent   ModelAgentService
	machine MachineService
}
