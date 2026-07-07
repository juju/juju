// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package safemode

import (
	"maps"

	"github.com/juju/clock"
	"github.com/juju/worker/v5/dependency"
	"github.com/prometheus/client_golang/prometheus"

	internallogger "github.com/juju/juju/internal/logger"
	"github.com/juju/juju/internal/worker/controlleragentconfig"
	"github.com/juju/juju/internal/worker/dbaccessor"
	"github.com/juju/juju/internal/worker/querylogger"
	"github.com/juju/juju/internal/worker/terminationworker"
)

// ManifoldsConfig allows specialisation of the result of Manifolds.
type ManifoldsConfig struct {
	// NewDBWorkerFunc returns a tracked db worker.
	NewDBWorkerFunc dbaccessor.NewDBWorkerFunc

	// ControllerStartupValues provides the controller-local startup values
	// needed by dbaccessor.
	ControllerStartupValues dbaccessor.ControllerStartupValuesProvider

	// ControllerID is the numeric ID of the controller.
	ControllerID string

	// LogDir is the controller process log directory.
	LogDir string

	// ConfigChangeSocketPath is the path to the config-change reload socket.
	ConfigChangeSocketPath string

	// Clock supplies timekeeping services to various workers.
	Clock clock.Clock
}

// commonManifolds returns manifolds shared between IAAS and CAAS
// controller safe-mode engines.  The controller binary is always a
// controller, so no ifController gating is required.
//
// Thou Shalt Not Use String Literals In This Function. Or Else.
func commonManifolds(config ManifoldsConfig) dependency.Manifolds {
	return dependency.Manifolds{
		// The termination worker returns ErrTerminateAgent if a
		// termination signal is received by the process it's running in.
		terminationName: terminationworker.Manifold(),

		// Controller agent config manifold watches the controller agent
		// config socket and bounces if it changes.
		controllerAgentConfigName: controlleragentconfig.Manifold(
			controlleragentconfig.ManifoldConfig{
				ControllerID:      config.ControllerID,
				Logger:            internallogger.GetLogger("juju.worker.controlleragentconfig"),
				NewSocketListener: controlleragentconfig.NewSocketListener,
				SocketName:        config.ConfigChangeSocketPath,
			},
		),

		// The query logger records slow or failing SQL queries.
		queryLoggerName: querylogger.Manifold(querylogger.ManifoldConfig{
			LogDir: config.LogDir,
			Logger: internallogger.GetLogger("juju.worker.querylogger"),
		}),
	}
}

// IAASManifolds returns manifolds for an IAAS controller safe-mode engine.
func IAASManifolds(config ManifoldsConfig) dependency.Manifolds {
	return mergeManifolds(config, dependency.Manifolds{
		dbAccessorName: dbaccessor.Manifold(dbaccessor.ManifoldConfig{
			QueryLoggerName:           queryLoggerName,
			ControllerAgentConfigName: controllerAgentConfigName,
			ControllerStartupValues:   config.ControllerStartupValues,
			Logger:                    internallogger.GetLogger("juju.worker.dbaccessor"),
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
	return mergeManifolds(config, dependency.Manifolds{
		dbAccessorName: dbaccessor.Manifold(dbaccessor.ManifoldConfig{
			QueryLoggerName:           queryLoggerName,
			ControllerAgentConfigName: controllerAgentConfigName,
			ControllerStartupValues:   config.ControllerStartupValues,
			Logger:                    internallogger.GetLogger("juju.worker.dbaccessor"),
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
