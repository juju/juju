// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage

import (
	"context"

	"github.com/juju/names/v6"
	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	apiservertesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/core/machine"
	coremodel "github.com/juju/juju/core/model"
	modeltesting "github.com/juju/juju/core/model/testing"
	"github.com/juju/juju/core/unit"
	"github.com/juju/juju/internal/uuid"
)

type baseStorageSuite struct {
	authorizer apiservertesting.FakeAuthorizer

	controllerUUID string
	modelUUID      coremodel.UUID

	api *StorageAPI

	unitTag    names.UnitTag
	machineTag names.MachineTag

	applicationService *MockApplicationService
	blockDeviceService *MockBlockDeviceService
	removalService     *MockRemovalService
	storageService     *MockStorageService

	poolsInUse []string
}

func (s *baseStorageSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.unitTag = names.NewUnitTag("mysql/0")
	s.machineTag = names.NewMachineTag("1234")

	s.authorizer = apiservertesting.FakeAuthorizer{Tag: names.NewUserTag("admin"), Controller: true}

	s.applicationService = NewMockApplicationService(ctrl)
	s.applicationService.EXPECT().GetUnitMachineName(gomock.Any(), unit.Name("mysql/0")).DoAndReturn(func(ctx context.Context, u unit.Name) (machine.Name, error) {
		c.Assert(u.String(), tc.Equals, s.unitTag.Id())
		return machine.Name(s.machineTag.Id()), nil
	}).AnyTimes()

	s.blockDeviceService = NewMockBlockDeviceService(ctrl)
	s.removalService = NewMockRemovalService(ctrl)
	s.storageService = NewMockStorageService(ctrl)

	s.poolsInUse = []string{}

	s.controllerUUID = uuid.MustNewUUID().String()
	s.modelUUID = modeltesting.GenModelUUID(c)

	s.api = NewStorageAPI(
		s.controllerUUID,
		s.modelUUID,
		s.authorizer,
		s.applicationService,
		s.blockDeviceService,
		s.removalService,
		s.storageService,
	)

	c.Cleanup(func() {
		s.api = nil
		s.applicationService = nil
		s.blockDeviceService = nil
		s.removalService = nil
		s.storageService = nil
	})

	return ctrl
}
