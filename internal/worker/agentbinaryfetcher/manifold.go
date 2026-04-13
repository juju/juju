// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package agentbinaryfetcher

import (
	"context"

	"github.com/juju/worker/v5"
	"github.com/juju/worker/v5/dependency"

	coredependency "github.com/juju/juju/core/dependency"
	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/core/logger"
	errors "github.com/juju/juju/internal/errors"
	"github.com/juju/juju/internal/services"
)

// NewWorkerFunc is the type of function that creates a new tools version
// updater worker.
type NewWorkerFunc func(WorkerConfig) worker.Worker

// ManifoldConfig defines the configuration for the tools version updater
// manifold.
type ManifoldConfig struct {
	DomainServicesName string

	GetDomainServices func(context.Context, dependency.Getter, string) (domainServices, error)
	NewWorker         NewWorkerFunc

	Logger logger.Logger
}

// Validate validates the manifold configuration.
func (cfg *ManifoldConfig) Validate() error {
	if cfg.DomainServicesName == "" {
		return errors.Errorf("empty DomainServicesName").Add(coreerrors.NotValid)
	}
	if cfg.Logger == nil {
		return errors.Errorf("nil Logger").Add(coreerrors.NotValid)
	}
	if cfg.GetDomainServices == nil {
		return errors.Errorf("nil GetDomainServices").Add(coreerrors.NotValid)
	}
	if cfg.NewWorker == nil {
		return errors.Errorf("nil NewWorker").Add(coreerrors.NotValid)
	}
	return nil
}

// Manifold returns a dependency manifold for the tools version updater worker.
func Manifold(cfg ManifoldConfig) dependency.Manifold {
	return dependency.Manifold{
		Inputs: []string{
			cfg.DomainServicesName,
		},
		Start: func(ctx context.Context, getter dependency.Getter) (worker.Worker, error) {
			if err := cfg.Validate(); err != nil {
				return nil, errors.Capture(err)
			}

			domainServices, err := cfg.GetDomainServices(ctx, getter, cfg.DomainServicesName)
			if err != nil {
				return nil, errors.Capture(err)
			}

			return cfg.NewWorker(WorkerConfig{
				ModelAgentService:  domainServices.modelAgent,
				AgentBinaryService: domainServices.agentBinary,
				Logger:             cfg.Logger,
			}), nil
		},
	}
}

// GetModelDomainServices returns the model domain services.
func GetModelDomainServices(ctx context.Context, getter dependency.Getter, domainServicesName string) (domainServices, error) {
	services, err := coredependency.GetDependencyByName(getter, domainServicesName, func(s services.DomainServices) services.DomainServices {
		return s
	})
	if err != nil {
		return domainServices{}, errors.Capture(err)
	}

	return domainServices{
		modelAgent:  services.Agent(),
		agentBinary: services.AgentBinary(),
	}, nil
}

type domainServices struct {
	modelAgent  ModelAgentService
	agentBinary AgentBinaryService
}
