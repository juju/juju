// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package migrationminion

import (
	"context"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/worker/v5"
	"github.com/juju/worker/v5/dependency"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/api"
	migrationminionapi "github.com/juju/juju/api/agent/migrationminion"
	"github.com/juju/juju/api/base"
	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/internal/worker/fortress"
)

// NewWorkerFunc is a function that returns a Worker backed by config, or an
// error.
type NewWorkerFunc func(config Config) (worker.Worker, error)

// ManifoldConfig defines the names of the manifolds on which a
// Worker manifold will depend.
type ManifoldConfig struct {
	AgentName         string
	APICallerName     string
	FortressName      string
	Clock             clock.Clock
	APIOpen           func(context.Context, *api.Info, api.DialOpts) (api.Connection, error)
	ValidateMigration func(context.Context, base.APICaller) error

	NewWorker             NewWorkerFunc
	SendReport            SendReportFunc
	FetchTargetLokiConfig LokiConfigFetcherFunc

	Logger logger.Logger
}

// Validate is called by start to check for bad configuration.
func (config ManifoldConfig) Validate() error {
	if config.AgentName == "" {
		return errors.NotValidf("empty AgentName")
	}
	if config.APICallerName == "" {
		return errors.NotValidf("empty APICallerName")
	}
	if config.FortressName == "" {
		return errors.NotValidf("empty FortressName")
	}
	if config.Clock == nil {
		return errors.NotValidf("nil Clock")
	}
	if config.APIOpen == nil {
		return errors.NotValidf("nil APIOpen")
	}
	if config.ValidateMigration == nil {
		return errors.NotValidf("nil ValidateMigration")
	}
	if config.NewWorker == nil {
		return errors.NotValidf("nil NewWorker")
	}
	if config.Logger == nil {
		return errors.NotValidf("nil Logger")
	}
	if config.SendReport == nil {
		return errors.NotValidf("nil SendReport")
	}
	if config.FetchTargetLokiConfig == nil {
		return errors.NotValidf("nil FetchTargetLokiConfig")
	}
	return nil
}

// start is a StartFunc for a Worker manifold.
func (config ManifoldConfig) start(context context.Context, getter dependency.Getter) (worker.Worker, error) {
	if err := config.Validate(); err != nil {
		return nil, errors.Trace(err)
	}
	var agent agent.Agent
	if err := getter.Get(config.AgentName, &agent); err != nil {
		return nil, errors.Trace(err)
	}
	var apiCaller base.APICaller
	if err := getter.Get(config.APICallerName, &apiCaller); err != nil {
		return nil, errors.Trace(err)
	}
	var guard fortress.Guard
	if err := getter.Get(config.FortressName, &guard); err != nil {
		return nil, errors.Trace(err)
	}

	worker, err := config.NewWorker(Config{
		Agent:                 agent,
		Facade:                migrationminionapi.NewClient(apiCaller),
		Guard:                 guard,
		Clock:                 config.Clock,
		APIOpen:               config.APIOpen,
		ValidateMigration:     config.ValidateMigration,
		Logger:                config.Logger,
		SendReport:            config.SendReport,
		FetchTargetLokiConfig: config.FetchTargetLokiConfig,
		ApplyJitter:           true,
	})
	if err != nil {
		return nil, errors.Trace(err)
	}
	return worker, nil
}

// Manifold returns a dependency manifold that runs the migration
// worker.
func Manifold(config ManifoldConfig) dependency.Manifold {
	return dependency.Manifold{
		Inputs: []string{
			config.AgentName,
			config.APICallerName,
			config.FortressName,
		},
		Start: config.start,
	}
}
