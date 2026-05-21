// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package dbrepl provides the dependency manifolds for the controller
// database REPL subcommand.  The controller binary is always a
// controller node, so none of the manifolds here need an ifController
// guard or a state-config-watcher input.
package dbrepl

import (
	"io"
	"maps"
	"path"

	"github.com/juju/clock"
	"github.com/juju/utils/v4/voyeur"
	"github.com/juju/worker/v5/dependency"

	coreagent "github.com/juju/juju/agent"
	internallogger "github.com/juju/juju/internal/logger"
	"github.com/juju/juju/internal/worker/agent"
	"github.com/juju/juju/internal/worker/controlleragentconfig"
	"github.com/juju/juju/internal/worker/dbrepl"
	"github.com/juju/juju/internal/worker/dbreplaccessor"
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

	// NewDBReplWorkerFunc returns a tracked db worker.
	NewDBReplWorkerFunc dbreplaccessor.NewDBReplWorkerFunc

	// Clock supplies timekeeping services to various workers.
	Clock clock.Clock

	// Stdout is the writer to use for stdout.
	Stdout io.Writer

	// Stderr is the writer to use for stderr.
	Stderr io.Writer

	// Stdin is the reader to use for stdin.
	Stdin io.Reader
}

// commonManifolds returns manifolds shared between IAAS and CAAS
// controller REPL engines.  The controller binary is always a
// controller, so no ifController gating is required.
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

		// The db-repl manifold drives the interactive REPL worker.
		dbReplName: dbrepl.Manifold(dbrepl.ManifoldConfig{
			DBReplAccessorName: dbReplAccessorName,
			Logger:             internallogger.GetLogger("juju.worker.dbrepl"),
			Stdout:             config.Stdout,
			Stderr:             config.Stderr,
			Stdin:              config.Stdin,
		}),
	}
}

// IAASManifolds returns manifolds for an IAAS controller REPL engine.
func IAASManifolds(config ManifoldsConfig) dependency.Manifolds {
	return mergeManifolds(config, dependency.Manifolds{
		dbReplAccessorName: dbreplaccessor.Manifold(dbreplaccessor.ManifoldConfig{
			AgentName:       agentName,
			Clock:           config.Clock,
			Logger:          internallogger.GetLogger("juju.worker.dbreplaccessor"),
			NewApp:          dbreplaccessor.NewApp,
			NewDBReplWorker: config.NewDBReplWorkerFunc,
			NewNodeManager:  dbreplaccessor.IAASNodeManager,
		}),
	})
}

// CAASManifolds returns manifolds for a CAAS controller REPL engine.
func CAASManifolds(config ManifoldsConfig) dependency.Manifolds {
	return mergeManifolds(config, dependency.Manifolds{
		dbReplAccessorName: dbreplaccessor.Manifold(dbreplaccessor.ManifoldConfig{
			AgentName:       agentName,
			Clock:           config.Clock,
			Logger:          internallogger.GetLogger("juju.worker.dbreplaccessor"),
			NewApp:          dbreplaccessor.NewApp,
			NewDBReplWorker: config.NewDBReplWorkerFunc,
			NewNodeManager:  dbreplaccessor.CAASNodeManager,
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

	dbReplName         = "db-repl"
	dbReplAccessorName = "db-repl-accessor"
)
