// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasapplicationprovisioner

import (
	stdtesting "testing"

	"github.com/juju/tc"
)

//go:generate go run go.uber.org/mock/mockgen -typed -package mocks -destination mocks/broker_mock.go github.com/juju/juju/internal/worker/caasapplicationprovisioner CAASBroker
//go:generate go run go.uber.org/mock/mockgen -typed -package mocks -destination mocks/facade_mock.go github.com/juju/juju/internal/worker/caasapplicationprovisioner CAASProvisionerFacade
//go:generate go run go.uber.org/mock/mockgen -typed -package mocks -destination mocks/unitfacade_mock.go github.com/juju/juju/internal/worker/caasapplicationprovisioner CAASUnitProvisionerFacade
//go:generate go run go.uber.org/mock/mockgen -typed -package mocks -destination mocks/runner_mock.go github.com/juju/juju/internal/worker/caasapplicationprovisioner Runner
//go:generate go run go.uber.org/mock/mockgen -typed -package mocks -destination mocks/ops_mock.go github.com/juju/juju/internal/worker/caasapplicationprovisioner ApplicationOps


var NewProvisionerWorkerForTest = newProvisionerWorker
var AppOps = &applicationOps{}
