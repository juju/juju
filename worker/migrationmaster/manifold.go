// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package migrationmaster

import (
	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/worker/v2"
	"github.com/juju/worker/v2/dependency"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/api"
	"github.com/juju/juju/api/base"
	"github.com/juju/juju/migration"
	"github.com/juju/juju/worker/fortress"
)

// logger is here to stop the desire of creating a package level logger.
// Don't do this, instead pass one passed as manifold config.
var logger interface{}

// ManifoldConfig defines the names of the manifolds on which a
// Worker manifold will depend.
type ManifoldConfig struct {
	AgentName     string
	APICallerName string
	FortressName  string

	Clock     clock.Clock
	NewFacade func(base.APICaller) (Facade, error)
	NewWorker func(Config) (worker.Worker, error)
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
	if config.NewFacade == nil {
		return errors.NotValidf("nil NewFacade")
	}
	if config.NewWorker == nil {
		return errors.NotValidf("nil NewWorker")
	}
	if config.Clock == nil {
		return errors.NotValidf("nil Clock")
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
	var apiConn api.Connection
	if err := context.Get(config.APICallerName, &apiConn); err != nil {
		return nil, errors.Trace(err)
	}
	var guard fortress.Guard
	if err := context.Get(config.FortressName, &guard); err != nil {
		return nil, errors.Trace(err)
	}
	facade, err := config.NewFacade(apiConn)
	if err != nil {
		return nil, errors.Trace(err)
	}
	apiClient := apiConn.Client()
	worker, err := config.NewWorker(Config{
		ModelUUID:       agent.CurrentConfig().Model().Id(),
		Facade:          facade,
		Guard:           guard,
		APIOpen:         api.Open,
		UploadBinaries:  migration.UploadBinaries,
		CharmDownloader: apiClient,
		ToolsDownloader: apiClient,
		Clock:           config.Clock,
	})
	if err != nil {
		return nil, errors.Trace(err)
	}
	return worker, nil
}

func errorFilter(err error) error {
	switch errors.Cause(err) {
	case ErrMigrated:
		// If the model has migrated, the migrationmaster should no
		// longer be running.
		return dependency.ErrUninstall
	case ErrInactive:
		// If the migration is no longer active, restart the
		// migrationmaster immediately so it can wait for the next
		// attempt.
		return dependency.ErrBounce
	default:
		return err
	}
}

// Manifold packages a Worker for use in a dependency.Engine.
func Manifold(config ManifoldConfig) dependency.Manifold {
	return dependency.Manifold{
		Inputs: []string{
			config.AgentName,
			config.APICallerName,
			config.FortressName,
		},
		Start:  config.start,
		Filter: errorFilter,
	}
}
