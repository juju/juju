// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasoperator

import (
	"github.com/juju/utils/clock"
	"github.com/prometheus/client_golang/prometheus"
	"gopkg.in/juju/worker.v1"

	coreagent "github.com/juju/juju/agent"
	"github.com/juju/juju/api"
	"github.com/juju/juju/cmd/jujud/agent/engine"
	"github.com/juju/juju/worker/agent"
	"github.com/juju/juju/worker/apicaller"
	"github.com/juju/juju/worker/caasoperator"
	"github.com/juju/juju/worker/dependency"
	"github.com/juju/juju/worker/logsender"
)

// ManifoldsConfig allows specialisation of the result of Manifolds.
type ManifoldsConfig struct {

	// Agent contains the agent that will be wrapped and made available to
	// its dependencies via a dependency.Engine.
	Agent coreagent.Agent

	// Clock contains the clock that will be made available to manifolds.
	Clock clock.Clock

	// LogSource will be read from by the logsender component.
	LogSource logsender.LogRecordCh

	// PrometheusRegisterer is a prometheus.Registerer that may be used
	// by workers to register Prometheus metric collectors.
	PrometheusRegisterer prometheus.Registerer
}

// Manifolds returns a set of co-configured manifolds covering the various
// responsibilities of a caasoperator agent. It also accepts the logSource
// argument because we haven't figured out how to thread all the logging bits
// through a dependency engine yet.
//
// Thou Shalt Not Use String Literals In This Function. Or Else.
func Manifolds(config ManifoldsConfig) dependency.Manifolds {

	return dependency.Manifolds{

		// The agent manifold references the enclosing agent, and is the
		// foundation stone on which most other manifolds ultimately depend.
		agentName: agent.Manifold(config.Agent),

		apiCallerName: apicaller.Manifold(apicaller.ManifoldConfig{
			AgentName:     agentName,
			APIOpen:       api.Open,
			NewConnection: apicaller.OnlyConnect,
		}),

		clockName: clockManifold(config.Clock),

		// The operator installs and deploys charm containers;
		// manages the unit's presence in its relations;
		// creates suboordinate units; runs all the hooks;
		// sends metrics; etc etc etc.

		operatorName: caasoperator.Manifold(caasoperator.ManifoldConfig{
			AgentName:     agentName,
			APICallerName: apiCallerName,
			ClockName:     clockName,
			NewWorker:     caasoperator.NewWorker,
		}),
	}
}

func clockManifold(clock clock.Clock) dependency.Manifold {
	return dependency.Manifold{
		Start: func(dependency.Context) (worker.Worker, error) {
			return engine.NewValueWorker(clock)
		},
		Output: engine.ValueWorkerOutput,
	}
}

const (
	agentName     = "agent"
	apiCallerName = "api-caller"
	clockName     = "clock"
	operatorName  = "operator"
)
