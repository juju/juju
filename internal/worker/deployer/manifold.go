// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package deployer

import (
	"context"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/names/v6"
	"github.com/juju/worker/v5"
	"github.com/juju/worker/v5/dependency"

	"github.com/juju/juju/agent"
	apideployer "github.com/juju/juju/api/agent/deployer"
	"github.com/juju/juju/api/base"
	"github.com/juju/juju/core/flightrecorder"
	corehttp "github.com/juju/juju/core/http"
	"github.com/juju/juju/core/logger"
)

// ManifoldConfig defines the names of the manifolds on which a Manifold will depend.
type ManifoldConfig struct {
	AgentName      string
	APICallerName  string
	HTTPClientName string
	FlightRecorder flightrecorder.FlightRecorder
	Clock          clock.Clock
	Logger         logger.Logger

	UnitEngineConfig func() dependency.EngineConfig
	SetupLogging     func(logger.LoggerContext, agent.Config)
	NewDeployContext func(ContextConfig) (Context, error)
}

// TODO: add ManifoldConfig.Validate.

// Manifold returns a dependency manifold that runs a deployer worker,
// using the resource names defined in the supplied config.
func Manifold(config ManifoldConfig) dependency.Manifold {
	return dependency.Manifold{
		Inputs: []string{
			config.AgentName,
			config.APICallerName,
			config.HTTPClientName,
		},
		Start: config.start,
	}
}

func (config ManifoldConfig) start(ctx context.Context, getter dependency.Getter) (worker.Worker, error) {
	var agent agent.Agent
	if err := getter.Get(config.AgentName, &agent); err != nil {
		return nil, err
	}
	var apiCaller base.APICaller
	if err := getter.Get(config.APICallerName, &apiCaller); err != nil {
		return nil, err
	}
	var httpClientGetter corehttp.HTTPClientGetter
	if err := getter.Get(config.HTTPClientName, &httpClientGetter); err != nil {
		return nil, err
	}
	return config.newWorker(ctx, agent, apiCaller, httpClientGetter)
}

// newWorker trivially wraps NewDeployer.
//
// It's not tested at the moment, because the scaffolding
// necessary is too unwieldy/distracting to introduce at this point.
func (config ManifoldConfig) newWorker(_ context.Context, a agent.Agent, apiCaller base.APICaller, httpClientGetter corehttp.HTTPClientGetter) (worker.Worker, error) {
	// TODO: run config.Validate()
	cfg := a.CurrentConfig()
	// Grab the tag and ensure that it's for a machine.
	if cfg.Tag().Kind() != names.MachineTagKind {
		return nil, errors.New("agent's tag is not a machine tag")
	}

	deployerFacade := apideployer.NewClient(apiCaller)
	contextConfig := ContextConfig{
		Agent:            a,
		FlightRecorder:   config.FlightRecorder,
		Clock:            config.Clock,
		Logger:           config.Logger,
		HTTPClientGetter: httpClientGetter,
		UnitEngineConfig: config.UnitEngineConfig,
		SetupLogging:     config.SetupLogging,
		UnitManifolds:    UnitManifolds,
	}

	context, err := config.NewDeployContext(contextConfig)
	if err != nil {
		return nil, errors.Trace(err)
	}
	shim := &facadeShim{st: deployerFacade}
	w, err := NewDeployer(shim, config.Logger, context)
	if err != nil {
		return nil, errors.Annotate(err, "cannot start unit agent deployer worker")
	}
	return w, nil
}

type facadeShim struct {
	st *apideployer.Client
}

func (s *facadeShim) Machine(tag names.MachineTag) (Machine, error) {
	// Need to deal with typed nils.
	machine, err := s.st.Machine(tag)
	if err != nil {
		return nil, err
	}
	return machine, nil
}

func (s *facadeShim) Unit(ctx context.Context, tag names.UnitTag) (Unit, error) {
	unit, err := s.st.Unit(ctx, tag)
	if err != nil {
		return nil, err
	}
	return unit, nil
}
