// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package safemode

import (
	"context"
	"path"

	"github.com/juju/clock"
	"github.com/juju/utils/v4/voyeur"
	"github.com/juju/version/v2"
	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/dependency"
	"github.com/prometheus/client_golang/prometheus"

	coreagent "github.com/juju/juju/agent"
	"github.com/juju/juju/agent/engine"
	"github.com/juju/juju/cmd/jujud-controller/util"
	corelogger "github.com/juju/juju/core/logger"
	internallogger "github.com/juju/juju/internal/logger"
	"github.com/juju/juju/internal/worker/agent"
	"github.com/juju/juju/internal/worker/controlleragentconfig"
	"github.com/juju/juju/internal/worker/dbaccessor"
	"github.com/juju/juju/internal/worker/logsender"
	"github.com/juju/juju/internal/worker/querylogger"
	"github.com/juju/juju/internal/worker/stateconfigwatcher"
	"github.com/juju/juju/internal/worker/terminationworker"
)

// ManifoldsConfig allows specialisation of the result of Manifolds.
type ManifoldsConfig struct {
	// AgentName is the name of the machine agent, like "machine-12".
	// This will never change during the execution of an agent, and
	// is used to provide this as config into a worker rather than
	// making the worker get it from the agent worker itself.
	AgentName string

	// Agent contains the agent that will be wrapped and made available to
	// its dependencies via a dependency.Engine.
	Agent coreagent.Agent

	// AgentConfigChanged is set whenever the machine agent's config
	// is updated.
	AgentConfigChanged *voyeur.Value

	// RootDir is the root directory that any worker that needs to
	// access local filesystems should use as a base. In actual use it
	// will be "" but it may be overridden in tests.
	RootDir string

	// PreviousAgentVersion passes through the version the machine
	// agent was running before the current restart.
	PreviousAgentVersion version.Number

	// NewDBWorkerFunc returns a tracked db worker.
	NewDBWorkerFunc dbaccessor.NewDBWorkerFunc

	// LogSource defines the channel type used to send log message
	// structs within the machine agent.
	LogSource logsender.LogRecordCh

	// Clock supplies timekeeping services to various workers.
	Clock clock.Clock

	// IsCaasConfig is true if this config is for a caas agent.
	IsCaasConfig bool

	// SetupLogging is used by the deployer to initialize the logging
	// context for the unit.
	SetupLogging func(corelogger.LoggerContext, coreagent.Config)
}

// commonManifolds returns a set of co-configured manifolds covering the
// various responsibilities of a machine agent.
//
// Thou Shalt Not Use String Literals In This Function. Or Else.
func commonManifolds(config ManifoldsConfig) dependency.Manifolds {
	agentConfig := config.Agent.CurrentConfig()

	manifolds := dependency.Manifolds{
		// The agent manifold references the enclosing agent, and is the
		// foundation stone on which most other manifolds ultimately depend.
		agentName: agent.Manifold(config.Agent),

		// The termination worker returns ErrTerminateAgent if a
		// termination signal is received by the process it's running
		// in. It has no inputs and its only output is the error it
		// returns. It depends on the uninstall file having been
		// written *by the manual provider* at install time; it would
		// be Very Wrong Indeed to use SetCanUninstall in conjunction
		// with this code.
		terminationName: terminationworker.Manifold(),

		clockName: clockManifold(config.Clock),

		// Each machine agent has a flag manifold/worker which
		// reports whether or not the agent is a controller.
		isControllerFlagName: util.IsControllerFlagManifold(stateConfigWatcherName, true),

		// Controller agent config manifold watches the controller
		// agent config and bounces if it changes.
		controllerAgentConfigName: ifController(controlleragentconfig.Manifold(controlleragentconfig.ManifoldConfig{
			AgentName:         agentName,
			Clock:             config.Clock,
			Logger:            internallogger.GetLogger("juju.worker.controlleragentconfig"),
			NewSocketListener: controlleragentconfig.NewSocketListener,
			SocketName:        path.Join(agentConfig.DataDir(), "configchange.socket"),
		})),

		// The stateconfigwatcher manifold watches the machine agent's
		// configuration and reports if state serving info is
		// present. It will bounce itself if state serving info is
		// added or removed. It is intended as a dependency just for
		// the state manifold.
		stateConfigWatcherName: stateconfigwatcher.Manifold(stateconfigwatcher.ManifoldConfig{
			AgentName:          agentName,
			AgentConfigChanged: config.AgentConfigChanged,
		}),

		dbAccessorName: ifController(dbaccessor.Manifold(dbaccessor.ManifoldConfig{
			AgentName:                 agentName,
			QueryLoggerName:           queryLoggerName,
			ControllerAgentConfigName: controllerAgentConfigName,
			Clock:                     config.Clock,
			Logger:                    internallogger.GetLogger("juju.worker.dbaccessor"),
			LogDir:                    agentConfig.LogDir(),
			PrometheusRegisterer:      noopPrometheusRegisterer{},
			NewApp:                    dbaccessor.NewApp,
			NewDBWorker:               config.NewDBWorkerFunc,
			NewMetricsCollector:       dbaccessor.NewMetricsCollector,
		})),

		queryLoggerName: ifController(querylogger.Manifold(querylogger.ManifoldConfig{
			LogDir: agentConfig.LogDir(),
			Clock:  config.Clock,
			Logger: internallogger.GetLogger("juju.worker.querylogger"),
		})),
	}

	return manifolds
}

// IAASManifolds returns a set of co-configured manifolds covering the
// various responsibilities of a IAAS machine agent.
func IAASManifolds(config ManifoldsConfig) dependency.Manifolds {
	return commonManifolds(config)
}

// CAASManifolds returns a set of co-configured manifolds covering the
// various responsibilities of a CAAS machine agent.
func CAASManifolds(config ManifoldsConfig) dependency.Manifolds {
	return commonManifolds(config)
}

func clockManifold(clock clock.Clock) dependency.Manifold {
	return dependency.Manifold{
		Start: func(_ context.Context, _ dependency.Getter) (worker.Worker, error) {
			return engine.NewValueWorker(clock)
		},
		Output: engine.ValueWorkerOutput,
	}
}

var ifController = engine.Housing{
	Flags: []string{
		isControllerFlagName,
	},
}.Decorate

const (
	agentName              = "agent"
	terminationName        = "termination-signal-handler"
	stateConfigWatcherName = "state-config-watcher"
	clockName              = "clock"

	isControllerFlagName      = "is-controller-flag"
	controllerAgentConfigName = "controller-agent-config"

	dbAccessorName  = "db-accessor"
	queryLoggerName = "query-logger"
)

type noopPrometheusRegisterer struct{}

// Register registers a new Collector to be included in metrics
// collection. It returns an error if the descriptors provided by the
// Collector are invalid or if they — in combination with descriptors of
// already registered Collectors — do not fulfill the consistency and
// uniqueness criteria described in the documentation of metric.Desc.
//
// If the provided Collector is equal to a Collector already registered
// (which includes the case of re-registering the same Collector), the
// returned error is an instance of AlreadyRegisteredError, which
// contains the previously registered Collector.
//
// A Collector whose Describe method does not yield any Desc is treated
// as unchecked. Registration will always succeed. No check for
// re-registering (see previous paragraph) is performed. Thus, the
// caller is responsible for not double-registering the same unchecked
// Collector, and for providing a Collector that will not cause
// inconsistent metrics on collection. (This would lead to scrape
// errors.)
func (noopPrometheusRegisterer) Register(prometheus.Collector) error { return nil }

// MustRegister works like Register but registers any number of
// Collectors and panics upon the first registration that causes an
// error.
func (noopPrometheusRegisterer) MustRegister(...prometheus.Collector) {}

// Unregister unregisters the Collector that equals the Collector passed
// in as an argument.  (Two Collectors are considered equal if their
// Describe method yields the same set of descriptors.) The function
// returns whether a Collector was unregistered. Note that an unchecked
// Collector cannot be unregistered (as its Describe method does not
// yield any descriptor).
//
// Note that even after unregistering, it will not be possible to
// register a new Collector that is inconsistent with the unregistered
// Collector, e.g. a Collector collecting metrics with the same name but
// a different help string. The rationale here is that the same registry
// instance must only collect consistent metrics throughout its
// lifetime.
func (noopPrometheusRegisterer) Unregister(prometheus.Collector) bool { return false }
