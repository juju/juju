// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storageprovisioner

//go:generate go run go.uber.org/mock/mockgen -typed -package storageprovisioner -destination watcher_mock_test.go github.com/juju/juju/core/watcher StringsWatcher,MachineStorageIDsWatcher
//go:generate go run go.uber.org/mock/mockgen -typed -package storageprovisioner_test -destination blockdevice_mock_test.go github.com/juju/juju/apiserver/facades/agent/storageprovisioner BlockDeviceService
//go:generate go run go.uber.org/mock/mockgen -typed -package storageprovisioner -destination storage_mock_test.go github.com/juju/juju/apiserver/facades/agent/storageprovisioner StorageBackend
//go:generate go run go.uber.org/mock/mockgen -typed -package storageprovisioner -destination state_mock_test.go github.com/juju/juju/state FilesystemAttachment,VolumeAttachment,Lifer
//go:generate go run go.uber.org/mock/mockgen -typed -package storageprovisioner -destination facade_mock_test.go github.com/juju/juju/apiserver/facade Resources,FacadeRegistry
//go:generate go run go.uber.org/mock/mockgen -typed -package storageprovisioner -destination service_mock_test.go github.com/juju/juju/apiserver/facades/agent/storageprovisioner ApplicationService,MachineService,StorageProvisioningService

func ptr[T any](v T) *T {
	return &v
}
