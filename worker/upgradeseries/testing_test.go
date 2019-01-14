// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.
package upgradeseries_test

import (
	"gopkg.in/juju/names.v2"
	"gopkg.in/juju/worker.v1"

	"github.com/golang/mock/gomock"
	"github.com/juju/juju/api/base"
	workermocks "github.com/juju/juju/worker/mocks"
	"github.com/juju/juju/worker/upgradeseries"
	. "github.com/juju/juju/worker/upgradeseries/mocks"
)

// validManifoldConfig returns a valid manifold config created from mocks based
// on the incoming controller. The mocked facade and worker are returned.
func validManifoldConfig(ctrl *gomock.Controller) (upgradeseries.ManifoldConfig, upgradeseries.Facade, worker.Worker) {
	facade := NewMockFacade(ctrl)
	work := workermocks.NewMockWorker(ctrl)

	cfg := newManifoldConfig(
		voidLogger(ctrl),
		func(_ base.APICaller, _ names.Tag) upgradeseries.Facade { return facade },
		func(_ upgradeseries.Config) (worker.Worker, error) { return work, nil },
	)

	return cfg, facade, work
}

// newManifoldConfig creates and returns a new ManifoldConfig instance based on
// the supplied arguments.
func newManifoldConfig(
	logger upgradeseries.Logger,
	newFacade func(base.APICaller, names.Tag) upgradeseries.Facade,
	newWorker func(upgradeseries.Config) (worker.Worker, error),
) upgradeseries.ManifoldConfig {
	return upgradeseries.ManifoldConfig{
		AgentName:     "agent-name",
		APICallerName: "api-caller-name",
		NewFacade:     newFacade,
		NewWorker:     newWorker,
		Logger:        logger,
	}
}

// voidLogger creates a new mock Logger that with no call verification.
func voidLogger(ctrl *gomock.Controller) upgradeseries.Logger {
	log := NewMockLogger(ctrl)

	exp := log.EXPECT()
	any := gomock.Any()
	exp.Debugf(any, any).AnyTimes()
	exp.Infof(any, any).AnyTimes()
	exp.Warningf(any, any).AnyTimes()
	exp.Errorf(any, any).AnyTimes()

	return log
}
