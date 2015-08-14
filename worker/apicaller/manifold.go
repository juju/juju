// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apicaller

import (
	"github.com/juju/errors"

	coreagent "github.com/juju/juju/agent"
	"github.com/juju/juju/api/base"
	"github.com/juju/juju/worker"
	"github.com/juju/juju/worker/agent"
	"github.com/juju/juju/worker/dependency"
	"github.com/juju/juju/worker/util"
)

// ManifoldConfig defines the names of the manifolds on which a Manifold will depend.
type ManifoldConfig util.AgentManifoldConfig

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
		var agent agent.Agent
		if err := getResource(config.AgentName, &agent); err != nil {
			return nil, err
		}
		conn, err := openConnection(agent)
		if err != nil {
			return nil, errors.Annotate(err, "cannot open api")
		}

		// Add the environment uuid to agent config if not present.
		currentConfig := agent.CurrentConfig()
		if currentConfig.Environment().Id() == "" {
			err := agent.ChangeConfig(func(setter coreagent.ConfigSetter) error {
				environTag, err := conn.EnvironTag()
				if err != nil {
					return errors.Annotate(err, "no environment uuid set on api")
				}
				return setter.Migrate(coreagent.MigrateParams{
					Environment: environTag,
				})
			})
			if err != nil {
				logger.Warningf("unable to save environment uuid: %v", err)
				// Not really fatal, just annoying.
			}
		}

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
