// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apicaller

import (
	"github.com/juju/errors"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/api/base"
	"github.com/juju/juju/worker"
	"github.com/juju/juju/worker/dependency"
	"github.com/juju/juju/worker/gate"
)

// ManifoldConfig defines the names of the manifolds on which a Manifold will depend.
type ManifoldConfig struct {
	AgentName       string
	APIInfoGateName string
}

// Manifold returns a manifold whose worker wraps an API connection made on behalf of
// the dependency identified by AgentName.
func Manifold(config ManifoldConfig) dependency.Manifold {
	return dependency.Manifold{
		Inputs: []string{
			config.AgentName,
			config.APIInfoGateName,
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
		var gate gate.Unlocker
		if err := getResource(config.APIInfoGateName, &gate); err != nil {
			return nil, err
		}
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
		if currentConfig.Environment().Id() == "" {
			err := a.ChangeConfig(func(setter agent.ConfigSetter) error {
				environTag, err := conn.EnvironTag()
				if err != nil {
					return errors.Annotate(err, "no environment uuid set on api")
				}
				return setter.Migrate(agent.MigrateParams{
					Environment: environTag,
				})
			})
			if err != nil {
				logger.Warningf("unable to save environment uuid: %v", err)
				// Not really fatal, just annoying.
			}
		}

		// Now we know the agent config has been fixed up, notify everyone
		// else who might depend upon its stability/correctness.
		gate.Unlock()

		// Return the worker.
		return newApiConnWorker(conn)
	}
}

// outputFunc extracts a base.APICaller from a *apiConnWorker.
func outputFunc(in worker.Worker, out interface{}) error {
	inWorker, _ := in.(*apiConnWorker)
	outPointer, _ := out.(*base.APICaller)
	if inWorker == nil || outPointer == nil {
		return errors.Errorf("expected %T->%T; got %T->%T", inWorker, outPointer, in, out)
	}
	*outPointer = inWorker.conn
	return nil
}
