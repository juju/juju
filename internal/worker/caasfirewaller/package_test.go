// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasfirewaller

import (
	"github.com/juju/errors"
	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/catacomb"
)

//go:generate go run go.uber.org/mock/mockgen -typed -package mocks -destination mocks/broker_mock.go github.com/juju/juju/internal/worker/caasfirewaller CAASBroker,PortMutator,ServiceUpdater
//go:generate go run go.uber.org/mock/mockgen -typed -package mocks -destination mocks/worker_mock.go github.com/juju/worker/v4 Worker
//go:generate go run go.uber.org/mock/mockgen -typed -package mocks -destination mocks/domain_mocks.go github.com/juju/juju/internal/worker/caasfirewaller ApplicationService,PortService
//go:generate go run go.uber.org/mock/mockgen -typed -package mocks -destination mocks/services_mocks.go github.com/juju/juju/internal/services ModelDomainServices

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
		Name: "caas-firewaller",
		Site: &p.catacomb,
		Work: p.loop,
	})
	return p, err
}
