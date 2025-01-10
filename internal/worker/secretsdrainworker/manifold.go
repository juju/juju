// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package secretsdrainworker

import (
	"context"

	"github.com/juju/errors"
	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/dependency"

	agentsecretsdrain "github.com/juju/juju/api/agent/secretsdrain"
	"github.com/juju/juju/api/agent/secretsmanager"
	"github.com/juju/juju/api/base"
	"github.com/juju/juju/api/controller/usersecretsdrain"
	"github.com/juju/juju/core/leadership"
	"github.com/juju/juju/core/logger"
	jujusecrets "github.com/juju/juju/internal/secrets"
)

// ManifoldConfig describes the resources used by the secretsdrainworker worker.
type ManifoldConfig struct {
	APICallerName         string
	LeadershipTrackerName string
	Logger                logger.Logger

	NewSecretsDrainFacade func(base.APICaller) SecretsDrainFacade
	NewWorker             func(Config) (worker.Worker, error)
	NewBackendsClient     func(base.APICaller) (jujusecrets.BackendsClient, error)
}

// NewSecretsDrainFacadeForAgent returns a new SecretsDrainFacade for draining charm owned secrets from agents.
func NewSecretsDrainFacadeForAgent(caller base.APICaller) SecretsDrainFacade {
	return agentsecretsdrain.NewClient(caller)
}

// NewUserSecretsDrainFacade returns a new SecretsDrainFacade for draining user secrets from controller.
func NewUserSecretsDrainFacade(caller base.APICaller) SecretsDrainFacade {
	return usersecretsdrain.NewClient(caller)
}

// NewSecretBackendsClientForAgent returns a new secret backends client for draining charm owned secrets from agents.
func NewSecretBackendsClientForAgent(caller base.APICaller) (jujusecrets.BackendsClient, error) {
	facade := secretsmanager.NewClient(caller)
	return jujusecrets.NewClient(facade)
}

// NewUserSecretBackendsClient returns a new secret backends client for draining user secrets from controller.
func NewUserSecretBackendsClient(caller base.APICaller) (jujusecrets.BackendsClient, error) {
	facade := usersecretsdrain.NewClient(caller)
	return jujusecrets.NewClient(facade)
}

// Manifold returns a Manifold that encapsulates the secretsdrainworker worker.
func Manifold(config ManifoldConfig) dependency.Manifold {
	inputs := []string{
		config.APICallerName,
	}
	// LeadershipTrackerName is optional.
	if config.LeadershipTrackerName != "" {
		inputs = append(inputs, config.LeadershipTrackerName)
	}
	return dependency.Manifold{
		Inputs: inputs,
		Start:  config.start,
	}
}

// Validate is called by start to check for bad configuration.
func (cfg ManifoldConfig) Validate() error {
	if cfg.APICallerName == "" {
		return errors.NotValidf("empty APICallerName")
	}
	if cfg.Logger == nil {
		return errors.NotValidf("nil Logger")
	}
	if cfg.NewSecretsDrainFacade == nil {
		return errors.NotValidf("nil NewSecretsDrainFacade")
	}
	if cfg.NewWorker == nil {
		return errors.NotValidf("nil NewWorker")
	}
	if cfg.NewBackendsClient == nil {
		return errors.NotValidf("nil NewBackendsClient")
	}
	return nil
}

// start is a StartFunc for a Worker manifold.
func (cfg ManifoldConfig) start(context context.Context, getter dependency.Getter) (worker.Worker, error) {
	if err := cfg.Validate(); err != nil {
		return nil, errors.Trace(err)
	}

	var apiCaller base.APICaller
	if err := getter.Get(cfg.APICallerName, &apiCaller); err != nil {
		return nil, errors.Trace(err)
	}

	// Drain worker used for user secrets and charm secrets.
	// Only the charm secrets worker needs the leadership tracker.
	var leadershipTracker leadership.ChangeTracker
	if cfg.LeadershipTrackerName == "" {
		leadershipTracker = passThroughLeadershipTracker{}
	} else {
		if err := getter.Get(cfg.LeadershipTrackerName, &leadershipTracker); err != nil {
			return nil, errors.Trace(err)
		}
	}
	leadershipTrackerFunc := func() leadership.ChangeTracker {
		return leadershipTracker
	}

	worker, err := cfg.NewWorker(Config{
		SecretsDrainFacade: cfg.NewSecretsDrainFacade(apiCaller),
		Logger:             cfg.Logger,
		SecretsBackendGetter: func() (jujusecrets.BackendsClient, error) {
			return cfg.NewBackendsClient(apiCaller)
		},
		LeadershipTrackerFunc: leadershipTrackerFunc,
	})
	if err != nil {
		return nil, errors.Trace(err)
	}
	return worker, nil
}

type passThroughLeadershipTracker struct{}

func (passThroughLeadershipTracker) WithStableLeadership(ctx context.Context, fn func(context.Context) error) error {
	return fn(ctx)
}
