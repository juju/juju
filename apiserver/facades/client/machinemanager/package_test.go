// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package machinemanager_test

import (
	"testing"

	coretesting "github.com/juju/juju/testing"
)

//go:generate go run go.uber.org/mock/mockgen -package mocks -destination mocks/leadership_mock.go github.com/juju/juju/apiserver/facades/client/machinemanager Leadership
//go:generate go run go.uber.org/mock/mockgen -package mocks -destination mocks/upgradebase_mock.go github.com/juju/juju/apiserver/facades/client/machinemanager Authorizer,UpgradeSeries,UpgradeSeriesState,UpgradeBaseValidator
//go:generate go run go.uber.org/mock/mockgen -package mocks -destination mocks/instance_config_mock.go github.com/juju/juju/apiserver/facades/client/machinemanager ControllerBackend,InstanceConfigBackend
//go:generate go run go.uber.org/mock/mockgen -package mocks -destination mocks/types_mock.go github.com/juju/juju/apiserver/facades/client/machinemanager Backend,StorageInterface,Pool,Model,Machine,Application,Unit,Charm,CharmhubClient
//go:generate go run go.uber.org/mock/mockgen -package mocks -destination mocks/domain_mock.go github.com/juju/juju/apiserver/facades/client/machinemanager ControllerConfigService
//go:generate go run go.uber.org/mock/mockgen -package mocks -destination mocks/state_mock.go github.com/juju/juju/state StorageAttachment,StorageInstance,Block
//go:generate go run go.uber.org/mock/mockgen -package mocks -destination mocks/state_storage_mock.go github.com/juju/juju/state/binarystorage StorageCloser
//go:generate go run go.uber.org/mock/mockgen -package mocks -destination mocks/volume_access_mock.go github.com/juju/juju/apiserver/common/storagecommon VolumeAccess
//go:generate go run go.uber.org/mock/mockgen -package mocks -destination mocks/environ_mock.go github.com/juju/juju/environs Environ
//go:generate go run go.uber.org/mock/mockgen -package mocks -destination mocks/objectstore_mock.go github.com/juju/juju/core/objectstore ObjectStore

func TestPackage(t *testing.T) {
	// TODO(wallyworld) - needed until instance config tests converted to gomock
	coretesting.MgoTestPackage(t)
}
