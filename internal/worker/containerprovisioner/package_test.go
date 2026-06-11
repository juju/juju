// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package containerprovisioner_test

//go:generate go run github.com/canonical/gomock/mockgen -package containerprovisioner_test -destination watcher_mock_test.go github.com/juju/juju/core/watcher StringsWatcher
//go:generate go run github.com/canonical/gomock/mockgen -package containerprovisioner_test -destination package_mock_test.go github.com/juju/juju/internal/worker/containerprovisioner ContainerMachine,ContainerMachineGetter,ContainerProvisionerAPI,ControllerAPI,MachinesAPI
