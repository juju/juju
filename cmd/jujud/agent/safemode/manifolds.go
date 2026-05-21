// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package safemode provides the dependency manifolds for the controller
// safe-mode recovery subcommand.  The controller binary is always a
// controller node, so none of the manifolds here need an ifController
// guard or a state-config-watcher input.
package safemode

import (
	"maps"
	"path"

	"github.com/juju/clock"
	"github.com/juju/utils/v4/voyeur"
	"github.com/juju/worker/v5/dependency"
	"github.com/prometheus/client_golang/prometheus"

	coreagent "github.com/juju/juju/agent"
	internallogger "github.com/juju/juju/internal/logger"
	"github.com/juju/juju/internal/worker/agent"
	"github.com/juju/juju/internal/worker/controlleragentconfig"
	"github.com/juju/juju/internal/worker/dbaccessor"
	"github.com/juju/juju/internal/worker/querylogger"
	"github.com/juju/juju/internal/worker/terminationworker"
)

// ManifoldsConfig allows specialisation of the result of Manifolds.
type ManifoldsConfig struct {
	// Agent contains the agent that will be wrapped and made available to
	// its dependencies via a dependency.Engine.
	Agent coreagent.Agent

	// AgentConfigChanged is set whenever the controller agent's config
	// is updated.
	AgentConfigChanged *voyeur.Value

	// NewDBWorkerFunc returns a tracked db worker.
	NewDBWorkerFunc dbaccessor.NewDBWorkerFunc

	// Clock supplies timekeeping services to various workers.
	Clock clock.Clock
}

// commonManifolds returns manifolds shared between IAAS and CAAS
// controller safe-mode engines.  The controller binary is always a
// controller, so no ifController gating is required.
//
// Thou Shalt Not Use String Literals In This Function. Or Else.
func commonManifolds(config ManifoldsConfig) dependency.Manifolds {
	agentConfig := config.Agent.CurrentConfig()

	return dependency.Manifolds{
		// The agent manifold references the enclosing agent, and is the
		// foundation stone on which most other manifolds ultimately
		// depend.
		agentName: agent.Manifold(config.Agent),

		// The termination worker returns ErrTerminateAgent if a
		// termination signal is received by the process it's running in.
		terminationName: terminationworker.Manifold(),

		// Controller agent config manifold watches the controller agent
		// config socket and bounces if it changes.
		controllerAgentConfigName: controlleragentconfig.Manifold(
			controlleragentconfig.ManifoldConfig{
				AgentName:         agentName,
				Clock:             config.Clock,
				Logger:            internallogger.GetLogger("juju.worker.controlleragentconfig"),
				NewSocketListener: controlleragentconfig.NewSocketListener,
				SocketName: path.Join(
					agentConfig.DataDir(), "configchange.socket",
				),
			},
		),

		// The query logger records slow or failing SQL queries.
		queryLoggerName: querylogger.Manifold(querylogger.ManifoldConfig{
			LogDir: agentConfig.LogDir(),
			Clock:  config.Clock,
			Logger: internallogger.GetLogger("juju.worker.querylogger"),
		}),
	}
}

// IAASManifolds returns manifolds for an IAAS controller safe-mode engine.
func IAASManifolds(config ManifoldsConfig) dependency.Manifolds {
	agentConfig := config.Agent.CurrentConfig()

	return mergeManifolds(config, dependency.Manifolds{
		dbAccessorName: dbaccessor.Manifold(dbaccessor.ManifoldConfig{
			AgentName:                 agentName,
			QueryLoggerName:           queryLoggerName,
			ControllerAgentConfigName: controllerAgentConfigName,
			Clock:                     config.Clock,
			Logger:                    internallogger.GetLogger("juju.worker.dbaccessor"),
			LogDir:                    agentConfig.LogDir(),
			PrometheusRegisterer:      noopPrometheusRegisterer{},
			NewApp:                    dbaccessor.NewApp,
			NewDBWorker:               config.NewDBWorkerFunc,
			NewNodeManager:            dbaccessor.IAASNodeManager,
			NewMetricsCollector:       dbaccessor.NewMetricsCollector,
		}),
	})
}

// CAASManifolds returns manifolds for a CAAS controller safe-mode engine.
func CAASManifolds(config ManifoldsConfig) dependency.Manifolds {
	agentConfig := config.Agent.CurrentConfig()

	return mergeManifolds(config, dependency.Manifolds{
		dbAccessorName: dbaccessor.Manifold(dbaccessor.ManifoldConfig{
			AgentName:                 agentName,
			QueryLoggerName:           queryLoggerName,
			ControllerAgentConfigName: controllerAgentConfigName,
			Clock:                     config.Clock,
			Logger:                    internallogger.GetLogger("juju.worker.dbaccessor"),
			LogDir:                    agentConfig.LogDir(),
			PrometheusRegisterer:      noopPrometheusRegisterer{},
			NewApp:                    dbaccessor.NewApp,
			NewDBWorker:               config.NewDBWorkerFunc,
			NewNodeManager:            dbaccessor.CAASNodeManager,
			NewMetricsCollector:       dbaccessor.NewMetricsCollector,
		}),
	})
}

func mergeManifolds(
	config ManifoldsConfig, manifolds dependency.Manifolds,
) dependency.Manifolds {
	result := commonManifolds(config)
	maps.Copy(result, manifolds)
	return result
}

const (
	agentName       = "agent"
	terminationName = "termination-signal-handler"

	controllerAgentConfigName = "controller-agent-config"

	dbAccessorName  = "db-accessor"
	queryLoggerName = "query-logger"
)

// noopPrometheusRegisterer is a no-op prometheus registerer.
// Safe-mode is a recovery tool; no metrics are required.
type noopPrometheusRegisterer struct{}

func (noopPrometheusRegisterer) Register(prometheus.Collector) error  { return nil }
func (noopPrometheusRegisterer) MustRegister(...prometheus.Collector) {}
func (noopPrometheusRegisterer) Unregister(prometheus.Collector) bool { return false }
