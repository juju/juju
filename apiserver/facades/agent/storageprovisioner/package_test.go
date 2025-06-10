// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storageprovisioner_test

import (
	"os"
	stdtesting "testing"

	"github.com/juju/tc"

	"github.com/juju/juju/internal/testing"
	"github.com/juju/juju/rpc/params"
)

//go:generate go run go.uber.org/mock/mockgen -typed -package storageprovisioner_test -destination blockdevice_mock_test.go github.com/juju/juju/apiserver/facades/agent/storageprovisioner BlockDeviceService
//go:generate go run go.uber.org/mock/mockgen -typed -package storageprovisioner -destination storage_mock_test.go github.com/juju/juju/apiserver/facades/agent/storageprovisioner StorageBackend,Backend
//go:generate go run go.uber.org/mock/mockgen -typed -package storageprovisioner -destination state_mock_test.go github.com/juju/juju/state FilesystemAttachment,VolumeAttachment,EntityFinder,Lifer
//go:generate go run go.uber.org/mock/mockgen -typed -package storageprovisioner -destination facade_mock_test.go github.com/juju/juju/apiserver/facade Resources
//go:generate go run go.uber.org/mock/mockgen -typed -package storageprovisioner -destination service_mock_test.go github.com/juju/juju/apiserver/facades/agent/storageprovisioner ApplicationService

func TestMain(m *stdtesting.M) {
	os.Exit(func() int {
		defer testing.MgoTestMain()()
		return m.Run()
	}())
}

type storageSetUp interface {
	setupVolumes(c *tc.C)
	setupFilesystems(c *tc.C)
}

type byMachineAndEntity []params.MachineStorageId

func (b byMachineAndEntity) Len() int {
	return len(b)
}

func (b byMachineAndEntity) Less(i, j int) bool {
	if b[i].MachineTag == b[j].MachineTag {
		return b[i].AttachmentTag < b[j].AttachmentTag
	}
	return b[i].MachineTag < b[j].MachineTag
}

func (b byMachineAndEntity) Swap(i, j int) {
	b[i], b[j] = b[j], b[i]
}
