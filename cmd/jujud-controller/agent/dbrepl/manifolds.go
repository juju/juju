// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package dbrepl

import (
	"context"
	"io"
	"path"

	"github.com/juju/clock"
	"github.com/juju/utils/v4/voyeur"
	"github.com/juju/version/v2"
	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/dependency"

	coreagent "github.com/juju/juju/agent"
	"github.com/juju/juju/agent/engine"
	"github.com/juju/juju/cmd/jujud-controller/util"
	corelogger "github.com/juju/juju/core/logger"
	internallogger "github.com/juju/juju/internal/logger"
	"github.com/juju/juju/internal/worker/agent"
	"github.com/juju/juju/internal/worker/controlleragentconfig"
	"github.com/juju/juju/internal/worker/dbrepl"
	"github.com/juju/juju/internal/worker/dbreplaccessor"
	"github.com/juju/juju/internal/worker/logsender"
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

	// NewDBReplWorkerFunc returns a tracked db worker.
	NewDBReplWorkerFunc dbreplaccessor.NewDBReplWorkerFunc

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

	// Stdout is the writer to use for stdout.
	Stdout io.Writer

	// Stderr is the writer to use for stderr.
	Stderr io.Writer

	// Stdin is the reader to use for stdin.
	Stdin io.Reader
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

		// The stateconfigwatcher manifold watches the machine agent's
		// configuration and reports if state serving info is
		// present. It will bounce itself if state serving info is
		// added or removed. It is intended as a dependency just for
		// the state manifold.
		stateConfigWatcherName: stateconfigwatcher.Manifold(stateconfigwatcher.ManifoldConfig{
			AgentName:          agentName,
			AgentConfigChanged: config.AgentConfigChanged,
		}),

		// Controller agent config manifold watches the controller
		// agent config and bounces if it changes.
		controllerAgentConfigName: ifController(controlleragentconfig.Manifold(controlleragentconfig.ManifoldConfig{
			AgentName:         agentName,
			Clock:             config.Clock,
			Logger:            internallogger.GetLogger("juju.worker.controlleragentconfig"),
			NewSocketListener: controlleragentconfig.NewSocketListener,
			SocketName:        path.Join(agentConfig.DataDir(), "configchange.socket"),
		})),

		dbReplName: ifController(dbrepl.Manifold(dbrepl.ManifoldConfig{
			DBReplAccessorName: dbReplAccessorName,
			Clock:              config.Clock,
			Logger:             internallogger.GetLogger("juju.worker.dbrepl"),
			Stdout:             config.Stdout,
			Stderr:             config.Stderr,
			Stdin:              config.Stdin,
		})),
	}

	return manifolds
}

// IAASManifolds returns a set of co-configured manifolds covering the
// various responsibilities of a IAAS machine agent.
func IAASManifolds(config ManifoldsConfig) dependency.Manifolds {
	return mergeManifolds(config, dependency.Manifolds{
		dbReplAccessorName: ifController(dbreplaccessor.Manifold(dbreplaccessor.ManifoldConfig{
			AgentName:       agentName,
			Clock:           config.Clock,
			Logger:          internallogger.GetLogger("juju.worker.dbreplaccessor"),
			NewApp:          dbreplaccessor.NewApp,
			NewDBReplWorker: config.NewDBReplWorkerFunc,
			NewNodeManager:  dbreplaccessor.IAASNodeManager,
		})),
	})
}

// CAASManifolds returns a set of co-configured manifolds covering the
// various responsibilities of a CAAS machine agent.
func CAASManifolds(config ManifoldsConfig) dependency.Manifolds {
	return mergeManifolds(config, dependency.Manifolds{
		dbReplAccessorName: ifController(dbreplaccessor.Manifold(dbreplaccessor.ManifoldConfig{
			AgentName:       agentName,
			Clock:           config.Clock,
			Logger:          internallogger.GetLogger("juju.worker.dbreplaccessor"),
			NewApp:          dbreplaccessor.NewApp,
			NewDBReplWorker: config.NewDBReplWorkerFunc,
			NewNodeManager:  dbreplaccessor.CAASNodeManager,
		})),
	})
}

func mergeManifolds(config ManifoldsConfig, manifolds dependency.Manifolds) dependency.Manifolds {
	result := commonManifolds(config)
	for name, manifold := range manifolds {
		result[name] = manifold
	}
	return result
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

	dbReplName         = "db-repl"
	dbReplAccessorName = "db-repl-accessor"
)
