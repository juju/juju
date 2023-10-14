// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package machinemanager_test

import (
	"testing"

	gc "gopkg.in/check.v1"
)

//go:generate go run go.uber.org/mock/mockgen -package mocks -destination mocks/leadership_mock.go github.com/juju/juju/apiserver/facades/client/machinemanager Leadership
//go:generate go run go.uber.org/mock/mockgen -package mocks -destination mocks/upgradeseries_mock.go github.com/juju/juju/apiserver/facades/client/machinemanager Authorizer,UpgradeSeries,UpgradeSeriesState,UpgradeSeriesValidator
//go:generate go run go.uber.org/mock/mockgen -package mocks -destination mocks/instance_config_mock.go github.com/juju/juju/apiserver/facades/client/machinemanager ControllerBackend,InstanceConfigBackend
//go:generate go run go.uber.org/mock/mockgen -package mocks -destination mocks/types_mock.go github.com/juju/juju/apiserver/facades/client/machinemanager Backend,StorageInterface,Pool,Model,Machine,Application,Unit,Charm,CharmhubClient
//go:generate go run go.uber.org/mock/mockgen -package mocks -destination mocks/state_mock.go github.com/juju/juju/state StorageAttachment,StorageInstance,Block
//go:generate go run go.uber.org/mock/mockgen -package mocks -destination mocks/state_storage_mock.go github.com/juju/juju/state/binarystorage StorageCloser
//go:generate go run go.uber.org/mock/mockgen -package mocks -destination mocks/volume_access_mock.go github.com/juju/juju/apiserver/common/storagecommon VolumeAccess
//go:generate go run go.uber.org/mock/mockgen -package mocks -destination mocks/environ_mock.go github.com/juju/juju/environs Environ

func TestPackage(t *testing.T) {
	gc.TestingT(t)
}
