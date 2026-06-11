// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storageprovisioner

//go:generate go run github.com/canonical/gomock/mockgen -package storageprovisioner -destination watcher_mock_test.go github.com/juju/juju/core/watcher StringsWatcher,MachineStorageIDsWatcher
//go:generate go run github.com/canonical/gomock/mockgen -package storageprovisioner_test -destination blockdevice_mock_test.go github.com/juju/juju/apiserver/facades/agent/storageprovisioner BlockDeviceService
//go:generate go run github.com/canonical/gomock/mockgen -package storageprovisioner -destination facade_mock_test.go github.com/juju/juju/apiserver/facade FacadeRegistry
//go:generate go run github.com/canonical/gomock/mockgen -package storageprovisioner -destination service_mock_test.go github.com/juju/juju/apiserver/facades/agent/storageprovisioner ApplicationService,MachineService,StorageProvisioningService,BlockDeviceService,RemovalService
