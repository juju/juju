// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apicaller

import (
	"github.com/juju/errors"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/api"
	"github.com/juju/juju/api/base"
	"github.com/juju/juju/worker"
	"github.com/juju/juju/worker/dependency"
)

// ConnectFunc is responsible for making and validating an API connection
// on behalf of an agent.
type ConnectFunc func(agent.Agent, api.OpenFunc) (api.Connection, error)

// ManifoldConfig defines a Manifold's dependencies.
type ManifoldConfig struct {

	// AgentName is the name of the Agent resource that supplies
	// connection information.
	AgentName string

	// APIConfigWatcherName identifies a resource that will be
	// invalidated when api configuration changes. It's not really
	// fundamental, because it's not used directly, except to create
	// Inputs; it would be perfectly reasonable to wrap a Manifold
	// to report an extra Input instead.
	APIConfigWatcherName string

	// APIOpen is passed into NewConnection, and should be used to
	// create an API connection. You should probably just set it to
	// the local APIOpen func.
	APIOpen api.OpenFunc

	// NewConnection is responsible for getting a connection from an
	// agent, and may be responsible for other things that need to be
	// done before anyone else gets to see the connection.
	//
	// You should probably set it to ScaryConnect when running a
	// machine agent, and to OnlyConnect when running a model agent
	// (which doesn't have its own process). Unit agents should use
	// ScaryConnect at the moment; and probably switch to OnlyConnect
	// when they move into machine agent processes.
	NewConnection ConnectFunc

	// Filter is used to specialize responses to connection errors
	// made on behalf of different kinds of agent.
	Filter dependency.FilterFunc
}

// Manifold returns a manifold whose worker wraps an API connection
// made as configured.
func Manifold(config ManifoldConfig) dependency.Manifold {
	return dependency.Manifold{
		Inputs: []string{
			config.AgentName,
			config.APIConfigWatcherName,
		},
		Output: outputFunc,
		Start:  config.startFunc(),
		Filter: config.Filter,
	}
}

// startFunc returns a StartFunc that creates a connection based on the
// supplied manifold config and wraps it in a worker.
func (config ManifoldConfig) startFunc() dependency.StartFunc {
	return func(context dependency.Context) (worker.Worker, error) {
		var agent agent.Agent
		if err := context.Get(config.AgentName, &agent); err != nil {
			return nil, err
		}

		conn, err := config.NewConnection(agent, config.APIOpen)
		if errors.Cause(err) == ErrChangedPassword {
			return nil, dependency.ErrBounce
		} else if err != nil {
			return nil, errors.Annotate(err, "cannot open api")
		}
		return newApiConnWorker(conn), nil
	}
}

// outputFunc extracts an API connection from a *apiConnWorker.
func outputFunc(in worker.Worker, out interface{}) error {
	inWorker, _ := in.(*apiConnWorker)
	if inWorker == nil {
		return errors.Errorf("in should be a %T; got %T", inWorker, in)
	}

	switch outPointer := out.(type) {
	case *base.APICaller:
		*outPointer = inWorker.conn
	case *api.Connection:
		// Using api.Connection is strongly discouraged as consumers
		// of this API connection should not be able to close it. This
		// option is only available to support legacy upgrade steps.
		*outPointer = inWorker.conn
	default:
		return errors.Errorf("out should be *base.APICaller or *api.Connection; got %T", out)
	}
	return nil
}
