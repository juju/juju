// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package httpserver

import (
	ctx "context"
	"github.com/juju/juju/core/changestream"
	"github.com/juju/juju/domain"
	"github.com/juju/loggo"
	"github.com/juju/worker/v3"

	"github.com/juju/juju/controller"
	coredatabase "github.com/juju/juju/core/database"
	ccservice "github.com/juju/juju/domain/controllerconfig/service"
	ccstate "github.com/juju/juju/domain/controllerconfig/state"
)

// NewWorkerShim calls through to NewWorker, and exists only
// to adapt to the signature of ManifoldConfig.NewWorker.
func NewWorkerShim(config Config) (worker.Worker, error) {
	return NewWorker(config)
}

// GetControllerConfig gets the controller config from a *State - it
// exists so we can test the manifold without a StateSuite.
func GetControllerConfig(dbGetter changestream.WatchableDBGetter) (controller.Config, error) {
	ctrlConfigService := ccservice.NewService(
		ccstate.NewState(domain.NewTxnRunnerFactoryForNamespace(
			dbGetter.GetWatchableDB,
			coredatabase.ControllerNS,
		)),
		domain.NewWatcherFactory(
			func() (changestream.WatchableDB, error) {
				return dbGetter.GetWatchableDB(coredatabase.ControllerNS)
			},
			loggo.GetLogger("juju.worker.httpserver"),
		),
	)
	return ctrlConfigService.ControllerConfig(ctx.TODO())
}
