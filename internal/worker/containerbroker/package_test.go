// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package containerbroker_test

import (
	stdtesting "testing"

	"github.com/juju/tc"
)

//go:generate go run go.uber.org/mock/mockgen -typed -package mocks -destination mocks/worker_mock.go github.com/juju/worker/v4 Worker
//go:generate go run go.uber.org/mock/mockgen -typed -package mocks -destination mocks/dependency_mock.go github.com/juju/worker/v4/dependency Getter
//go:generate go run go.uber.org/mock/mockgen -typed -package mocks -destination mocks/environs_mock.go github.com/juju/juju/environs LXDProfiler,InstanceBroker
//go:generate go run go.uber.org/mock/mockgen -typed -package mocks -destination mocks/machine_lock_mock.go github.com/juju/juju/core/machinelock Lock
//go:generate go run go.uber.org/mock/mockgen -typed -package mocks -destination mocks/base_mock.go github.com/juju/juju/api/base APICaller
//go:generate go run go.uber.org/mock/mockgen -typed -package mocks -destination mocks/agent_mock.go github.com/juju/juju/agent Agent,Config

func TestPackage(t *stdtesting.T) {
	tc.TestingT(t)
}
