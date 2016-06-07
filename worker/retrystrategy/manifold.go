// Copyright 2016 Canonical Ltd.
// Copyright 2016 Cloudbase Solutions SRL
// Licensed under the AGPLv3, see LICENCE file for details.

package retrystrategy

import (
	"github.com/juju/errors"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/api/base"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/cmd/jujud/agent/engine"
	"github.com/juju/juju/worker"
	"github.com/juju/juju/worker/dependency"
)

// ManifoldConfig defines the names of the manifolds on which a Manifold will depend.
type ManifoldConfig struct {
	AgentName     string
	APICallerName string
	NewFacade     func(base.APICaller) Facade
	NewWorker     func(WorkerConfig) (worker.Worker, error)
}

// Manifold returns a dependency manifold that runs a hook retry strategy worker,
// using the agent name and the api connection resources named in the supplied config.
func Manifold(config ManifoldConfig) dependency.Manifold {
	typedConfig := engine.AgentApiManifoldConfig{
		AgentName:     config.AgentName,
		APICallerName: config.APICallerName,
	}
	manifold := engine.AgentApiManifold(typedConfig, config.start)
	manifold.Output = config.output
	return manifold
}

func (mc ManifoldConfig) start(a agent.Agent, apiCaller base.APICaller) (worker.Worker, error) {
	agentTag := a.CurrentConfig().Tag()
	retryStrategyFacade := mc.NewFacade(apiCaller)
	initialRetryStrategy, err := retryStrategyFacade.RetryStrategy(agentTag)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return mc.NewWorker(WorkerConfig{
		Facade:        retryStrategyFacade,
		AgentTag:      agentTag,
		RetryStrategy: initialRetryStrategy,
	})
}

func (mc ManifoldConfig) output(in worker.Worker, out interface{}) error {
	inWorker, _ := in.(*RetryStrategyWorker)
	if inWorker == nil {
		return errors.Errorf("in should be a *retryStrategyWorker; is %T", in)
	}
	switch outPointer := out.(type) {
	case *params.RetryStrategy:
		*outPointer = inWorker.GetRetryStrategy()
	default:
		return errors.Errorf("out should be a *params.RetryStrategy; is %T", out)

	}
	return nil
}
