// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package introspection

import (
	"runtime"

	"github.com/juju/errors"
	"github.com/juju/loggo"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/worker"
	"github.com/juju/juju/worker/dependency"
)

var logger = loggo.GetLogger("juju.worker.introspection")

// ManifoldConfig describes the resources required to construct the
// introspection worker.
type ManifoldConfig struct {
	AgentName  string
	WorkerFunc func(Config) (worker.Worker, error)
}

// Manifold returns a Manifold which encapsulates the introspection worker.
func Manifold(config ManifoldConfig) dependency.Manifold {
	return dependency.Manifold{
		Inputs: []string{config.AgentName},
		Start: func(context dependency.Context) (worker.Worker, error) {
			// Since the worker listens on an abstract domain socket, this
			// is only available on linux.
			if runtime.GOOS != "linux" {
				logger.Debugf("introspection worker not supported on %q", runtime.GOOS)
				return nil, dependency.ErrUninstall
			}

			var a agent.Agent
			if err := context.Get(config.AgentName, &a); err != nil {
				return nil, errors.Trace(err)
			}

			socketName := "jujud-" + a.CurrentConfig().Tag().String()
			w, err := config.WorkerFunc(Config{
				SocketName: socketName,
			})
			if err != nil {
				return nil, errors.Trace(err)
			}
			return w, nil
		},
	}
}
