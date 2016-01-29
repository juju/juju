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

// ManifoldConfig defines the names of the manifolds on which a Manifold will depend.
type ManifoldConfig struct {
	AgentName string
}

// Manifold returns a manifold whose worker wraps an API connection made on behalf of
// the dependency identified by AgentName.
func Manifold(config ManifoldConfig) dependency.Manifold {
	return dependency.Manifold{
		Inputs: []string{
			config.AgentName,
		},
		Output: outputFunc,
		Start:  startFunc(config),
	}
}

// startFunc returns a StartFunc that creates a worker based on the manifolds
// named in the supplied config.
func startFunc(config ManifoldConfig) dependency.StartFunc {
	return func(getResource dependency.GetResourceFunc) (worker.Worker, error) {

		// Get dependencies and open a connection.
		var a agent.Agent
		if err := getResource(config.AgentName, &a); err != nil {
			return nil, err
		}
		conn, err := openConnection(a)
		if err != nil {
			return nil, errors.Annotate(err, "cannot open api")
		}

		// Add the environment uuid to agent config if not present.
		currentConfig := a.CurrentConfig()
		if currentConfig.Model().Id() == "" {
			err := a.ChangeConfig(func(setter agent.ConfigSetter) error {
				modelTag, err := conn.ModelTag()
				if err != nil {
					return errors.Annotate(err, "no model uuid set on api")
				}
				return setter.Migrate(agent.MigrateParams{
					Model: modelTag,
				})
			})
			if err != nil {
				logger.Warningf("unable to save model uuid: %v", err)
				// Not really fatal, just annoying.
			}
		}

		// Return the worker.
		return newApiConnWorker(conn)
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
