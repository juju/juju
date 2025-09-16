// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storageprovisioner

//go:generate go run go.uber.org/mock/mockgen -typed -package storageprovisioner -destination watcher_mock_test.go github.com/juju/juju/core/watcher StringsWatcher,MachineStorageIDsWatcher
//go:generate go run go.uber.org/mock/mockgen -typed -package storageprovisioner_test -destination blockdevice_mock_test.go github.com/juju/juju/apiserver/facades/agent/storageprovisioner BlockDeviceService
//go:generate go run go.uber.org/mock/mockgen -typed -package storageprovisioner -destination facade_mock_test.go github.com/juju/juju/apiserver/facade FacadeRegistry
//go:generate go run go.uber.org/mock/mockgen -typed -package storageprovisioner -destination service_mock_test.go github.com/juju/juju/apiserver/facades/agent/storageprovisioner ApplicationService,MachineService,StorageProvisioningService,BlockDeviceService

func ptr[T any](v T) *T {
	return &v
}
