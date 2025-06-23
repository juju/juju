// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package migrations

import (
	"github.com/juju/description/v9"
	"github.com/juju/errors"
	"github.com/juju/names/v5"
	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"
)

type ExternalControllersExportSuite struct{}

var _ = gc.Suite(&ExternalControllersExportSuite{})

func (s *ExternalControllersExportSuite) TestExportExternalController(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	entities := []MigrationRemoteApplication{
		s.migrationRemoteApplication(ctrl, func(expect *MockMigrationRemoteApplicationMockRecorder) {
			expect.SourceModel().Return(names.NewModelTag("uuid-2"))
		}),
	}

	extCtrlModel := s.migrationExternalController(ctrl, func(expect *MockMigrationExternalControllerMockRecorder) {
		expect.ID().Return("f47ac10b-58cc-4372-a567-0e02b2c3d479").Times(2)
		expect.Addrs().Return([]string{"10.0.0.1/24"})
		expect.Alias().Return("magic")
		expect.CACert().Return("magic-ca-cert")
		expect.Models().Return([]string{"xxxx-yyyy-zzzz"})
	})

	externalController := NewMockExternalController(ctrl)

	source := NewMockExternalControllerSource(ctrl)
	source.EXPECT().AllRemoteApplications().Return(entities, nil)
	source.EXPECT().ControllerForModel("uuid-2").Return(extCtrlModel, nil)

	model := NewMockExternalControllerModel(ctrl)
	model.EXPECT().AddExternalController(description.ExternalControllerArgs{
		Tag:    names.NewControllerTag("f47ac10b-58cc-4372-a567-0e02b2c3d479"),
		Addrs:  []string{"10.0.0.1/24"},
		Alias:  "magic",
		CACert: "magic-ca-cert",
		Models: []string{"xxxx-yyyy-zzzz"},
	}).Return(externalController)

	migration := ExportExternalControllers{}
	err := migration.Execute(source, model)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *ExternalControllersExportSuite) TestExportExternalControllerRequestsExternalControllerOnceWithSameUUID(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	entities := []MigrationRemoteApplication{
		s.migrationRemoteApplication(ctrl, func(expect *MockMigrationRemoteApplicationMockRecorder) {
			expect.SourceModel().Return(names.NewModelTag("uuid-2"))
		}),
		s.migrationRemoteApplication(ctrl, func(expect *MockMigrationRemoteApplicationMockRecorder) {
			expect.SourceModel().Return(names.NewModelTag("uuid-2"))
		}),
	}

	extCtrlModel := s.migrationExternalController(ctrl, func(expect *MockMigrationExternalControllerMockRecorder) {
		expect.ID().Return("f47ac10b-58cc-4372-a567-0e02b2c3d479").Times(2)
		expect.Addrs().Return([]string{"10.0.0.1/24"})
		expect.Alias().Return("magic")
		expect.CACert().Return("magic-ca-cert")
		expect.Models().Return([]string{"xxxx-yyyy-zzzz"})
	})

	externalController := NewMockExternalController(ctrl)

	source := NewMockExternalControllerSource(ctrl)
	source.EXPECT().AllRemoteApplications().Return(entities, nil)
	source.EXPECT().ControllerForModel("uuid-2").Return(extCtrlModel, nil)

	model := NewMockExternalControllerModel(ctrl)
	model.EXPECT().AddExternalController(description.ExternalControllerArgs{
		Tag:    names.NewControllerTag("f47ac10b-58cc-4372-a567-0e02b2c3d479"),
		Addrs:  []string{"10.0.0.1/24"},
		Alias:  "magic",
		CACert: "magic-ca-cert",
		Models: []string{"xxxx-yyyy-zzzz"},
	}).Return(externalController)

	migration := ExportExternalControllers{}
	err := migration.Execute(source, model)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *ExternalControllersExportSuite) TestExportExternalControllerRequestsExternalControllerOnceWithSameController(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	entities := []MigrationRemoteApplication{
		s.migrationRemoteApplication(ctrl, func(expect *MockMigrationRemoteApplicationMockRecorder) {
			expect.SourceModel().Return(names.NewModelTag("uuid-2"))
		}),
		s.migrationRemoteApplication(ctrl, func(expect *MockMigrationRemoteApplicationMockRecorder) {
			expect.SourceModel().Return(names.NewModelTag("uuid-3"))
		}),
	}

	extCtrlModel := s.migrationExternalController(ctrl, func(expect *MockMigrationExternalControllerMockRecorder) {
		expect.ID().Return("f47ac10b-58cc-4372-a567-0e02b2c3d479").Times(3)
		expect.Addrs().Return([]string{"10.0.0.1/24"})
		expect.Alias().Return("magic")
		expect.CACert().Return("magic-ca-cert")
		expect.Models().Return([]string{"xxxx-yyyy-zzzz"})
	})

	externalController := NewMockExternalController(ctrl)

	source := NewMockExternalControllerSource(ctrl)
	source.EXPECT().AllRemoteApplications().Return(entities, nil)
	source.EXPECT().ControllerForModel("uuid-2").Return(extCtrlModel, nil)
	source.EXPECT().ControllerForModel("uuid-3").Return(extCtrlModel, nil)

	model := NewMockExternalControllerModel(ctrl)
	model.EXPECT().AddExternalController(description.ExternalControllerArgs{
		Tag:    names.NewControllerTag("f47ac10b-58cc-4372-a567-0e02b2c3d479"),
		Addrs:  []string{"10.0.0.1/24"},
		Alias:  "magic",
		CACert: "magic-ca-cert",
		Models: []string{"xxxx-yyyy-zzzz"},
	}).Return(externalController)

	migration := ExportExternalControllers{}
	err := migration.Execute(source, model)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *ExternalControllersExportSuite) TestExportExternalControllerWithNoControllerNotFound(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	entities := []MigrationRemoteApplication{
		s.migrationRemoteApplication(ctrl, func(expect *MockMigrationRemoteApplicationMockRecorder) {
			expect.SourceModel().Return(names.NewModelTag("uuid-2"))
		}),
	}

	source := NewMockExternalControllerSource(ctrl)
	source.EXPECT().AllRemoteApplications().Return(entities, nil)
	source.EXPECT().ControllerForModel("uuid-2").Return(nil, errors.NotFoundf("not found"))

	model := NewMockExternalControllerModel(ctrl)

	migration := ExportExternalControllers{}
	err := migration.Execute(source, model)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *ExternalControllersExportSuite) TestExportExternalControllerFailsGettingRemoteApplicationEntities(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	source := NewMockExternalControllerSource(ctrl)
	source.EXPECT().AllRemoteApplications().Return(nil, errors.New("fail"))

	model := NewMockExternalControllerModel(ctrl)

	migration := ExportExternalControllers{}
	err := migration.Execute(source, model)
	c.Assert(err, gc.ErrorMatches, "fail")
}

func (s *ExternalControllersExportSuite) TestExportExternalControllerFailsGettingExternalControllerEntities(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	entities := []MigrationRemoteApplication{
		s.migrationRemoteApplication(ctrl, func(expect *MockMigrationRemoteApplicationMockRecorder) {
			expect.SourceModel().Return(names.NewModelTag("uuid-2"))
		}),
	}

	source := NewMockExternalControllerSource(ctrl)
	source.EXPECT().AllRemoteApplications().Return(entities, nil)
	source.EXPECT().ControllerForModel("uuid-2").Return(nil, errors.New("fail"))

	model := NewMockExternalControllerModel(ctrl)

	migration := ExportExternalControllers{}
	err := migration.Execute(source, model)
	c.Assert(err, gc.ErrorMatches, "fail")
}

func (s *ExternalControllersExportSuite) migrationExternalController(ctrl *gomock.Controller, fn func(expect *MockMigrationExternalControllerMockRecorder)) *MockMigrationExternalController {
	entity := NewMockMigrationExternalController(ctrl)
	fn(entity.EXPECT())
	return entity
}

func (s *ExternalControllersExportSuite) migrationRemoteApplication(ctrl *gomock.Controller, fn func(expect *MockMigrationRemoteApplicationMockRecorder)) *MockMigrationRemoteApplication {
	entity := NewMockMigrationRemoteApplication(ctrl)
	fn(entity.EXPECT())
	return entity
}
