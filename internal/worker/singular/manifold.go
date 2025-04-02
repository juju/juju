// Copyright 2015-2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package singular

import (
	"context"
	"time"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/names/v6"
	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/dependency"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/agent/engine"
	"github.com/juju/juju/core/lease"
	"github.com/juju/juju/core/model"
)

// ManifoldConfig holds the information necessary to run a FlagWorker in
// a dependency.Engine.
type ManifoldConfig struct {
	AgentName        string
	LeaseManagerName string

	Clock    clock.Clock
	Duration time.Duration
	// TODO(controlleragent) - claimaint should be a ControllerAgentTag
	Claimant names.Tag
	Entity   names.Tag

	NewWorker func(context.Context, FlagConfig) (worker.Worker, error)
}

// Validate ensures the required values are set.
func (config *ManifoldConfig) Validate() error {
	if config.AgentName == "" {
		return errors.NotValidf("empty AgentName")
	}
	if config.LeaseManagerName == "" {
		return errors.NotValidf("empty LeaseManagerName")
	}
	if config.Clock == nil {
		return errors.NotValidf("nil Clock")
	}
	if config.NewWorker == nil {
		return errors.NotValidf("nil NewWorker")
	}

	if !names.IsValidMachine(config.Claimant.Id()) && !names.IsValidControllerAgent(config.Claimant.Id()) {
		return errors.NotValidf("claimant tag")
	}

	return nil
}

// start is a method on ManifoldConfig because it's more readable than a closure.
func (config ManifoldConfig) start(ctx context.Context, getter dependency.Getter) (worker.Worker, error) {
	if err := config.Validate(); err != nil {
		return nil, errors.Trace(err)
	}

	var agent agent.Agent
	if err := getter.Get(config.AgentName, &agent); err != nil {
		return nil, errors.Trace(err)
	}

	modelUUID := model.UUID(agent.CurrentConfig().Model().Id())
	if err := modelUUID.Validate(); err != nil {
		return nil, errors.Trace(err)
	}

	var leaseManager lease.Manager
	if err := getter.Get(config.LeaseManagerName, &leaseManager); err != nil {
		return nil, errors.Trace(err)
	}

	flag, err := config.NewWorker(ctx, FlagConfig{
		Clock:        config.Clock,
		LeaseManager: leaseManager,
		Claimant:     config.Claimant,
		Entity:       config.Entity,
		ModelUUID:    modelUUID,
		Duration:     config.Duration,
	})
	if errors.Is(err, ErrRefresh) {
		return nil, dependency.ErrBounce
	} else if err != nil {
		return nil, errors.Trace(err)
	}
	return flag, nil
}

// Manifold returns a dependency.Manifold that will run a FlagWorker and
// expose it to clients as a engine.Flag resource.
func Manifold(config ManifoldConfig) dependency.Manifold {
	return dependency.Manifold{
		Inputs: []string{
			config.AgentName,
			config.LeaseManagerName,
		},
		Start: config.start,
		Output: func(in worker.Worker, out interface{}) error {
			return engine.FlagOutput(in, out)
		},
		Filter: func(err error) error {
			if errors.Is(err, ErrRefresh) {
				return dependency.ErrBounce
			}
			return err
		},
	}
}
