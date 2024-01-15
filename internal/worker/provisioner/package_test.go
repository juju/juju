// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provisioner_test

import (
	stdtesting "testing"

	gc "gopkg.in/check.v1"
)

//go:generate go run go.uber.org/mock/mockgen -package mocks -destination mocks/watcher.go github.com/juju/juju/core/watcher StringsWatcher
//go:generate go run go.uber.org/mock/mockgen -package mocks -destination mocks/provisioner.go github.com/juju/juju/internal/worker/provisioner ContainerMachine,ContainerMachineGetter,ContainerProvisionerAPI,ControllerAPI,MachinesAPI
//go:generate go run go.uber.org/mock/mockgen -package mocks -destination mocks/dependency.go github.com/juju/worker/v4/dependency Getter
//go:generate go run go.uber.org/mock/mockgen -package mocks -destination mocks/base_mock.go github.com/juju/juju/api/base APICaller

func TestPackage(t *stdtesting.T) {
	gc.TestingT(t)
}
