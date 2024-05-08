// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package machinemanager_test

import (
	"testing"

	coretesting "github.com/juju/juju/testing"
)

//go:generate go run go.uber.org/mock/mockgen -typed -package machinemanager_test -destination package_mock_test.go github.com/juju/juju/apiserver/facades/client/machinemanager Leadership,Authorizer,UpgradeSeries,UpgradeSeriesState,UpgradeBaseValidator,ControllerBackend,InstanceConfigBackend,Backend,StorageInterface,Pool,Model,Machine,Application,Unit,Charm,CharmhubClient,ControllerConfigService,MachineService,NetworkService
//go:generate go run go.uber.org/mock/mockgen -typed -package machinemanager_test -destination state_mock_test.go github.com/juju/juju/state StorageAttachment,StorageInstance,Block
//go:generate go run go.uber.org/mock/mockgen -typed -package machinemanager_test -destination state_storage_mock_test.go github.com/juju/juju/state/binarystorage StorageCloser
//go:generate go run go.uber.org/mock/mockgen -typed -package machinemanager_test -destination volume_access_mock_test.go github.com/juju/juju/apiserver/common/storagecommon VolumeAccess
//go:generate go run go.uber.org/mock/mockgen -typed -package machinemanager_test -destination environ_mock_test.go github.com/juju/juju/environs Environ
//go:generate go run go.uber.org/mock/mockgen -typed -package machinemanager_test -destination objectstore_mock_test.go github.com/juju/juju/core/objectstore ObjectStore
func TestPackage(t *testing.T) {
	// TODO(wallyworld) - needed until instance config tests converted to gomock
	coretesting.MgoTestPackage(t)
}
