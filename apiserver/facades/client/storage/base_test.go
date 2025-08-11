// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage_test

import (
	"context"

	"github.com/juju/names/v6"
	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/apiserver/facades/client/storage"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/core/machine"
	coremodel "github.com/juju/juju/core/model"
	modeltesting "github.com/juju/juju/core/model/testing"
	"github.com/juju/juju/core/unit"
	jujustorage "github.com/juju/juju/internal/storage"
	"github.com/juju/juju/internal/testhelpers"
	coretesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/internal/uuid"
)

type baseStorageSuite struct {
	coretesting.BaseSuite

	authorizer apiservertesting.FakeAuthorizer

	controllerUUID string
	modelUUID      coremodel.UUID

	api                 *storage.StorageAPI
	apiCaas             *storage.StorageAPI
	blockDeviceGetter   *mockBlockDeviceGetter
	blockCommandService *storage.MockBlockCommandService

	unitTag    names.UnitTag
	machineTag names.MachineTag

	stub testhelpers.Stub

	storageService     *storage.MockStorageService
	applicationService *storage.MockApplicationService
	registry           jujustorage.StaticProviderRegistry
	poolsInUse         []string
}

func (s *baseStorageSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.unitTag = names.NewUnitTag("mysql/0")
	s.machineTag = names.NewMachineTag("1234")

	s.authorizer = apiservertesting.FakeAuthorizer{Tag: names.NewUserTag("admin"), Controller: true}
	s.stub.ResetCalls()
	s.blockDeviceGetter = &mockBlockDeviceGetter{}

	s.blockCommandService = storage.NewMockBlockCommandService(ctrl)
	s.storageService = storage.NewMockStorageService(ctrl)
	s.applicationService = storage.NewMockApplicationService(ctrl)
	s.applicationService.EXPECT().GetUnitMachineName(gomock.Any(), unit.Name("mysql/0")).DoAndReturn(func(ctx context.Context, u unit.Name) (machine.Name, error) {
		c.Assert(u.String(), tc.Equals, s.unitTag.Id())
		return machine.Name(s.machineTag.Id()), nil
	}).AnyTimes()

	s.registry = jujustorage.StaticProviderRegistry{Providers: map[jujustorage.ProviderType]jujustorage.Provider{}}
	s.poolsInUse = []string{}

	s.controllerUUID = uuid.MustNewUUID().String()
	s.modelUUID = modeltesting.GenModelUUID(c)
	s.api = storage.NewStorageAPI(
		s.controllerUUID, s.modelUUID,
		s.blockDeviceGetter,
		s.storageService, s.applicationService, s.storageRegistryGetter,
		s.authorizer, s.blockCommandService)
	s.apiCaas = storage.NewStorageAPI(
		s.controllerUUID, s.modelUUID,
		s.blockDeviceGetter,
		s.storageService, s.applicationService, s.storageRegistryGetter,
		s.authorizer, s.blockCommandService)

	return ctrl
}

func (s *baseStorageSuite) storageRegistryGetter(context.Context) (jujustorage.ProviderRegistry, error) {
	return s.registry, nil
}
