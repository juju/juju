// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package migration_test

import (
	"testing"

	"github.com/juju/clock"
	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/core/model"
	"github.com/juju/juju/core/modelmigration"
	corestorage "github.com/juju/juju/core/storage"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/migration"
	"github.com/juju/juju/internal/storage"
	jujutesting "github.com/juju/juju/internal/testing"
)

type ExportImportSuite struct {
	controllerConfigService *MockControllerConfigService
	domainServices          *MockDomainServices
	domainServicesGetter    *MockDomainServicesGetter
	objectStoreGetter       *MockModelObjectStoreGetter
}

func TestExportImportSuite(t *testing.T) {
	tc.Run(t, &ExportImportSuite{})
}

func (s *ExportImportSuite) SetUpSuite(c *tc.C) {
	c.Skip(`
TODO tlm: We are skipping these tests as they are currently relying heavily on
mocks for how the importer is working internally. Now that we are trying to test
model migration into DQlite we have hit the problem where this can no longer be
an isolation suite and needs a full database. This is due to the fact that the
Setup call to the import operations construct their own services and they're not
something that can be injected as "mocks" from this layer.

I have added this to the risk register for 4.0 and will discuss further with
Simon. For the moment these tests can't continue as is due to DB dependencies
that are needed now.
`)
}

func (s *ExportImportSuite) exportImport(c *tc.C, leaders map[string]string) {
	bytes := []byte(modelYaml)
	scope := func(model.UUID) modelmigration.Scope { return modelmigration.NewScope(nil, nil, nil) }
	importer := migration.NewModelImporter(
		scope, s.controllerConfigService, s.domainServicesGetter,
		corestorage.ConstModelStorageRegistry(func() storage.ProviderRegistry {
			return nil
		}),
		s.objectStoreGetter,
		loggertesting.WrapCheckLog(c),
		clock.WallClock,
	)
	err := importer.ImportModel(c.Context(), bytes)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *ExportImportSuite) TestExportImportModel(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.exportImport(c, map[string]string{})
}

func (s *ExportImportSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.controllerConfigService = NewMockControllerConfigService(ctrl)
	s.controllerConfigService.EXPECT().ControllerConfig(gomock.Any()).Return(jujutesting.FakeControllerConfig(), nil).AnyTimes()

	s.domainServices = NewMockDomainServices(ctrl)
	s.domainServices.EXPECT().Cloud().Return(nil).AnyTimes()
	s.domainServices.EXPECT().Credential().Return(nil).AnyTimes()
	s.domainServices.EXPECT().Machine().Return(nil)
	s.domainServices.EXPECT().Application().Return(nil)
	s.domainServicesGetter = NewMockDomainServicesGetter(ctrl)
	s.domainServicesGetter.EXPECT().ServicesForModel(gomock.Any(), model.UUID("bd3fae18-5ea1-4bc5-8837-45400cf1f8f6")).Return(s.domainServices, nil)
	s.objectStoreGetter = NewMockModelObjectStoreGetter(ctrl)

	return ctrl
}
