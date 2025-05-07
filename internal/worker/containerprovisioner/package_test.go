// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package containerprovisioner_test

import (
	stdtesting "testing"

	"github.com/juju/tc"
)

//go:generate go run go.uber.org/mock/mockgen -typed -package containerprovisioner_test -destination watcher_mock_test.go github.com/juju/juju/core/watcher StringsWatcher
//go:generate go run go.uber.org/mock/mockgen -typed -package containerprovisioner_test -destination package_mock_test.go github.com/juju/juju/internal/worker/containerprovisioner ContainerMachine,ContainerMachineGetter,ContainerProvisionerAPI,ControllerAPI,MachinesAPI
func TestPackage(t *stdtesting.T) {
	tc.TestingT(t)
}
