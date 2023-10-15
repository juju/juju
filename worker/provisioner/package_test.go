// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provisioner_test

import (
	stdtesting "testing"

	"github.com/juju/juju/testing"
)

//go:generate go run go.uber.org/mock/mockgen -package mocks -destination mocks/watcher.go github.com/juju/juju/core/watcher StringsWatcher
//go:generate go run go.uber.org/mock/mockgen -package mocks -destination mocks/provisioner.go github.com/juju/juju/worker/provisioner ContainerMachine,ContainerMachineGetter,TaskAPI
//go:generate go run go.uber.org/mock/mockgen -package mocks -destination mocks/dependency.go github.com/juju/worker/v3/dependency Context

func Test(t *stdtesting.T) {
	testing.MgoTestPackage(t)
}
