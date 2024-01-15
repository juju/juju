// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasfirewaller

import (
	"testing"

	"github.com/juju/errors"
	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/catacomb"
	gc "gopkg.in/check.v1"
)

//go:generate go run go.uber.org/mock/mockgen -package mocks -destination mocks/broker_mock.go github.com/juju/juju/internal/worker/caasfirewaller CAASBroker,PortMutator,ServiceUpdater
//go:generate go run go.uber.org/mock/mockgen -package mocks -destination mocks/client_mock.go github.com/juju/juju/internal/worker/caasfirewaller Client,CAASFirewallerAPI,LifeGetter
//go:generate go run go.uber.org/mock/mockgen -package mocks -destination mocks/worker_mock.go github.com/juju/worker/v4 Worker
//go:generate go run go.uber.org/mock/mockgen -package mocks -destination mocks/api_base_mock.go github.com/juju/juju/api/base APICaller

func TestAll(t *testing.T) {
	gc.TestingT(t)
}

type (
	ApplicationWorkerCreator = applicationWorkerCreator
)

var (
	NewApplicationWorker = newApplicationWorker
)

func NewWorkerForTest(config Config, f ApplicationWorkerCreator) (worker.Worker, error) {
	if err := config.Validate(); err != nil {
		return nil, errors.Trace(err)
	}
	p := newFirewaller(config, f)
	err := catacomb.Invoke(catacomb.Plan{
		Site: &p.catacomb,
		Work: p.loop,
	})
	return p, err
}
