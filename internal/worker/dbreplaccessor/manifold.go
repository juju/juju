// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package dbreplaccessor

import (
	"context"
	"path"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/dependency"

	"github.com/juju/juju/agent"
	coredatabase "github.com/juju/juju/core/database"
	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/internal/database"
	"github.com/juju/juju/internal/worker/common"
)

// NewDBReplWorkerFunc creates a tracked db worker.
type NewDBReplWorkerFunc func(context.Context, DBApp, string, ...TrackedDBWorkerOption) (TrackedDB, error)

// NewNodeManagerFunc creates a NodeManager
type NewNodeManagerFunc func(agent.Config, logger.Logger, coredatabase.SlowQueryLogger) NodeManager

// ManifoldConfig contains:
// - The names of other manifolds on which the DB accessor depends.
// - Other dependencies from ManifoldsConfig required by the worker.
type ManifoldConfig struct {
	AgentName       string
	Clock           clock.Clock
	Logger          logger.Logger
	LogDir          string
	NewApp          NewAppFunc
	NewDBReplWorker NewDBReplWorkerFunc
	NewNodeManager  NewNodeManagerFunc
}

func (cfg ManifoldConfig) Validate() error {
	if cfg.AgentName == "" {
		return errors.NotValidf("empty AgentName")
	}
	if cfg.Clock == nil {
		return errors.NotValidf("nil Clock")
	}
	if cfg.Logger == nil {
		return errors.NotValidf("nil Logger")
	}
	if cfg.LogDir == "" {
		return errors.NotValidf("empty LogDir")
	}
	if cfg.NewApp == nil {
		return errors.NotValidf("nil NewApp")
	}
	if cfg.NewDBReplWorker == nil {
		return errors.NotValidf("nil NewDBReplWorker")
	}
	if cfg.NewNodeManager == nil {
		return errors.NotValidf("nil NewNodeManager")
	}
	return nil
}

// Manifold returns a dependency manifold that runs the dbaccessor
// worker, using the resource names defined in the supplied config.
func Manifold(config ManifoldConfig) dependency.Manifold {
	return dependency.Manifold{
		Inputs: []string{
			config.AgentName,
		},
		Output: dbAccessorOutput,
		Start: func(ctx context.Context, getter dependency.Getter) (worker.Worker, error) {
			if err := config.Validate(); err != nil {
				return nil, errors.Trace(err)
			}

			var thisAgent agent.Agent
			if err := getter.Get(config.AgentName, &thisAgent); err != nil {
				return nil, err
			}
			agentConfig := thisAgent.CurrentConfig()
			controllerID := agentConfig.Tag().Id()
			configPath := path.Join(agentConfig.DataDir(), "agents", "controller-"+controllerID, "controller.conf")
			controllerConf := controllerConfigReader{configPath: configPath}

			cfg := WorkerConfig{
				NodeManager:     config.NewNodeManager(agentConfig, config.Logger, coredatabase.NoopSlowQueryLogger{}),
				Clock:           config.Clock,
				ControllerID:    controllerID,
				Logger:          config.Logger,
				NewApp:          config.NewApp,
				NewDBReplWorker: config.NewDBReplWorker,
				ClusterConfig:   controllerConf,
			}

			return NewWorker(cfg)
		},
	}
}

func dbAccessorOutput(in worker.Worker, out interface{}) error {
	if w, ok := in.(*common.CleanupWorker); ok {
		in = w.Worker
	}
	w, ok := in.(*dbReplWorker)
	if !ok {
		return errors.Errorf("expected input of type dbWorker, got %T", in)
	}

	switch out := out.(type) {
	case *coredatabase.DBGetter:
		var target coredatabase.DBGetter = w
		*out = target
	default:
		return errors.Errorf("expected output of *database.DBGetter or *database.DBDeleter, got %T", out)
	}
	return nil
}

// IAASNodeManager returns a NodeManager that is configured to use
// the cloud-local TLS terminated address for Dqlite.
func IAASNodeManager(cfg agent.Config, logger logger.Logger, slowQueryLogger coredatabase.SlowQueryLogger) NodeManager {
	return database.NewNodeManager(cfg, false, logger, slowQueryLogger)
}

// CAASNodeManager returns a NodeManager that is configured to use
// the loopback address for Dqlite.
func CAASNodeManager(cfg agent.Config, logger logger.Logger, slowQueryLogger coredatabase.SlowQueryLogger) NodeManager {
	return database.NewNodeManager(cfg, true, logger, slowQueryLogger)
}
