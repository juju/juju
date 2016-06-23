// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package migrationmaster

import (
	"github.com/juju/errors"
	"github.com/juju/utils/clock"

	"github.com/juju/juju/api"
	"github.com/juju/juju/api/base"
	"github.com/juju/juju/migration"
	"github.com/juju/juju/worker"
	"github.com/juju/juju/worker/dependency"
	"github.com/juju/juju/worker/fortress"
)

// ManifoldConfig defines the names of the manifolds on which a
// Worker manifold will depend.
type ManifoldConfig struct {
	APICallerName string
	FortressName  string

	Clock     clock.Clock
	NewFacade func(base.APICaller) (Facade, error)
	NewWorker func(Config) (worker.Worker, error)
}

// validate is called by start to check for bad configuration.
func (config ManifoldConfig) validate() error {
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
	if err := config.validate(); err != nil {
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

// Manifold packages a Worker for use in a dependency.Engine.
func Manifold(config ManifoldConfig) dependency.Manifold {
	return dependency.Manifold{
		Inputs: []string{config.APICallerName, config.FortressName},
		Start:  config.start,
	}
}
