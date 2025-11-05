// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage

import (
	"github.com/juju/names/v6"
	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	apiservertesting "github.com/juju/juju/apiserver/testing"
	coremodel "github.com/juju/juju/core/model"
	modeltesting "github.com/juju/juju/core/model/testing"
	loggertesting "github.com/juju/juju/internal/logger/testing"
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
	removalService     *MockRemovalService
	storageService     *MockStorageService
}

func (s *baseStorageSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.unitTag = names.NewUnitTag("mysql/0")
	s.machineTag = names.NewMachineTag("1234")

	s.authorizer = apiservertesting.FakeAuthorizer{Tag: names.NewUserTag("admin"), Controller: true}

	s.applicationService = NewMockApplicationService(ctrl)
	s.removalService = NewMockRemovalService(ctrl)
	s.storageService = NewMockStorageService(ctrl)

	s.controllerUUID = uuid.MustNewUUID().String()
	s.modelUUID = modeltesting.GenModelUUID(c)

	s.api = NewStorageAPI(
		s.controllerUUID,
		s.modelUUID,
		s.authorizer,
		loggertesting.WrapCheckLog(c),
		s.applicationService,
		s.removalService,
		s.storageService,
	)

	c.Cleanup(func() {
		s.authorizer = apiservertesting.FakeAuthorizer{}
		s.api = nil
		s.applicationService = nil
		s.controllerUUID = ""
		s.machineTag = names.MachineTag{}
		s.modelUUID = ""
		s.removalService = nil
		s.storageService = nil
		s.unitTag = names.UnitTag{}
	})

	return ctrl
}
