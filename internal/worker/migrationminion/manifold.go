// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package migrationminion

import (
	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/worker/v3"
	"github.com/juju/worker/v3/dependency"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/api"
	"github.com/juju/juju/api/base"
	"github.com/juju/juju/internal/worker/fortress"
)

// Logger represents the methods used for logging messages.
type Logger interface {
	Errorf(string, ...interface{})
	Infof(string, ...interface{})
	Debugf(string, ...interface{})
	Warningf(string, ...interface{})
	Criticalf(string, ...interface{})
}

// ManifoldConfig defines the names of the manifolds on which a
// Worker manifold will depend.
type ManifoldConfig struct {
	AgentName         string
	APICallerName     string
	FortressName      string
	Clock             clock.Clock
	APIOpen           func(*api.Info, api.DialOpts) (api.Connection, error)
	ValidateMigration func(base.APICaller) error

	NewFacade func(base.APICaller) (Facade, error)
	NewWorker func(Config) (worker.Worker, error)
	Logger    Logger
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
	if config.NewFacade == nil {
		return errors.NotValidf("nil NewFacade")
	}
	if config.NewWorker == nil {
		return errors.NotValidf("nil NewWorker")
	}
	if config.Logger == nil {
		return errors.NotValidf("nil Logger")
	}
	return nil
}

// start is a StartFunc for a Worker manifold.
func (config ManifoldConfig) start(context dependency.Context) (worker.Worker, error) {
	if err := config.Validate(); err != nil {
		return nil, errors.Trace(err)
	}
	var agent agent.Agent
	if err := context.Get(config.AgentName, &agent); err != nil {
		return nil, errors.Trace(err)
	}
	var apiCaller base.APICaller
	if err := context.Get(config.APICallerName, &apiCaller); err != nil {
		return nil, errors.Trace(err)
	}
	var guard fortress.Guard
	if err := context.Get(config.FortressName, &guard); err != nil {
		return nil, errors.Trace(err)
	}

	facade, err := config.NewFacade(apiCaller)
	if err != nil {
		return nil, errors.Trace(err)
	}
	worker, err := config.NewWorker(Config{
		Agent:             agent,
		Facade:            facade,
		Guard:             guard,
		Clock:             config.Clock,
		APIOpen:           config.APIOpen,
		ValidateMigration: config.ValidateMigration,
		NewFacade:         config.NewFacade,
		Logger:            config.Logger,
		ApplyJitter:       true,
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
