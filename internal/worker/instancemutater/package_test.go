// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package instancemutater_test

import (
	"testing"

	gc "gopkg.in/check.v1"
)

//go:generate go run go.uber.org/mock/mockgen -package mocks -destination mocks/instancebroker_mock.go github.com/juju/juju/internal/worker/instancemutater InstanceMutaterAPI
//go:generate go run go.uber.org/mock/mockgen -package mocks -destination mocks/logger_mock.go github.com/juju/juju/internal/worker/instancemutater Logger
//go:generate go run go.uber.org/mock/mockgen -package mocks -destination mocks/machinemutater_mock.go github.com/juju/juju/api/agent/instancemutater MutaterMachine
//go:generate go run go.uber.org/mock/mockgen -package mocks -destination mocks/mutatercontext_mock.go github.com/juju/juju/internal/worker/instancemutater MutaterContext
//go:generate go run go.uber.org/mock/mockgen -package mocks -destination mocks/worker_mock.go github.com/juju/worker/v4 Worker
//go:generate go run go.uber.org/mock/mockgen -package mocks -destination mocks/dependency_mock.go github.com/juju/worker/v4/dependency Getter
//go:generate go run go.uber.org/mock/mockgen -package mocks -destination mocks/environs_mock.go github.com/juju/juju/environs Environ,LXDProfiler,InstanceBroker
//go:generate go run go.uber.org/mock/mockgen -package mocks -destination mocks/base_mock.go github.com/juju/juju/api/base APICaller
//go:generate go run go.uber.org/mock/mockgen -package mocks -destination mocks/agent_mock.go github.com/juju/juju/agent Agent,Config

func TestPackage(t *testing.T) {
	gc.TestingT(t)
}
