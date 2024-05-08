// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasapplicationprovisioner_test

import (
	jujutesting "github.com/juju/testing"
	"github.com/juju/worker/v4"
)

//go:generate go run go.uber.org/mock/mockgen -typed -package mocks -destination mocks/broker_mock.go github.com/juju/juju/internal/worker/caasapplicationprovisioner CAASBroker
//go:generate go run go.uber.org/mock/mockgen -typed -package mocks -destination mocks/facade_mock.go github.com/juju/juju/internal/worker/caasapplicationprovisioner CAASProvisionerFacade
//go:generate go run go.uber.org/mock/mockgen -typed -package mocks -destination mocks/unitfacade_mock.go github.com/juju/juju/internal/worker/caasapplicationprovisioner CAASUnitProvisionerFacade
//go:generate go run go.uber.org/mock/mockgen -typed -package mocks -destination mocks/runner_mock.go github.com/juju/juju/internal/worker/caasapplicationprovisioner Runner
//go:generate go run go.uber.org/mock/mockgen -typed -package mocks -destination mocks/ops_mock.go github.com/juju/juju/internal/worker/caasapplicationprovisioner ApplicationOps

type mockNotifyWorker struct {
	worker.Worker
	jujutesting.Stub
}

func (w *mockNotifyWorker) Notify() {
	w.MethodCall(w, "Notify")
}
