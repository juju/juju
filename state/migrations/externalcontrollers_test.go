// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package migrations

import (
	"github.com/juju/description/v3"
	"github.com/juju/errors"
	"github.com/juju/names/v4"
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

func (s *ExternalControllersExportSuite) TestExportExternalControllerWithNoControllerNotFoundModelIsLocal(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	entities := []MigrationRemoteApplication{
		s.migrationRemoteApplication(ctrl, func(expect *MockMigrationRemoteApplicationMockRecorder) {
			expect.SourceModel().Return(names.NewModelTag("uuid-2"))
		}),
	}

	localCtrl := s.migrationExternalController(ctrl, func(expect *MockMigrationExternalControllerMockRecorder) {
		expect.ID().Return("local-ctrl-uuid").Times(2)
		expect.Addrs().Return([]string{"10.0.0.1:17070"})
		expect.Alias().Return("my-controller")
		expect.CACert().Return("local-ca-cert")
		expect.Models().Return([]string{"uuid-2"})
	})

	externalController := NewMockExternalController(ctrl)

	source := NewMockExternalControllerSource(ctrl)
	source.EXPECT().AllRemoteApplications().Return(entities, nil)
	source.EXPECT().ControllerForModel("uuid-2").Return(nil, errors.NotFoundf("not found"))
	source.EXPECT().ModelExists("uuid-2").Return(true, nil)
	source.EXPECT().LocalControllerInfo([]string{"uuid-2"}).Return(localCtrl, nil)

	model := NewMockExternalControllerModel(ctrl)
	model.EXPECT().AddExternalController(description.ExternalControllerArgs{
		Tag:    names.NewControllerTag("local-ctrl-uuid"),
		Addrs:  []string{"10.0.0.1:17070"},
		Alias:  "my-controller",
		CACert: "local-ca-cert",
		Models: []string{"uuid-2"},
	}).Return(externalController)

	migration := ExportExternalControllers{}
	err := migration.Execute(source, model)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *ExternalControllersExportSuite) TestExportExternalControllerWithNoControllerNotFoundModelNotLocal(c *gc.C) {
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
	source.EXPECT().ModelExists("uuid-2").Return(false, nil)

	model := NewMockExternalControllerModel(ctrl)

	migration := ExportExternalControllers{}
	err := migration.Execute(source, model)
	c.Assert(err, gc.ErrorMatches,
		`cannot find external controller for model "uuid-2" and model is not on this controller: not found not found`)
}

func (s *ExternalControllersExportSuite) TestExportExternalControllerMultipleLocalModels(c *gc.C) {
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

	localCtrl := s.migrationExternalController(ctrl, func(expect *MockMigrationExternalControllerMockRecorder) {
		expect.ID().Return("local-ctrl-uuid").Times(2)
		expect.Addrs().Return([]string{"10.0.0.1:17070"})
		expect.Alias().Return("my-controller")
		expect.CACert().Return("local-ca-cert")
		expect.Models().Return([]string{"uuid-2", "uuid-3"})
	})

	externalController := NewMockExternalController(ctrl)

	source := NewMockExternalControllerSource(ctrl)
	source.EXPECT().AllRemoteApplications().Return(entities, nil)
	source.EXPECT().ControllerForModel("uuid-2").Return(nil, errors.NotFoundf("not found"))
	source.EXPECT().ModelExists("uuid-2").Return(true, nil)
	source.EXPECT().ControllerForModel("uuid-3").Return(nil, errors.NotFoundf("not found"))
	source.EXPECT().ModelExists("uuid-3").Return(true, nil)
	source.EXPECT().LocalControllerInfo(gomock.Any()).Return(localCtrl, nil)

	model := NewMockExternalControllerModel(ctrl)
	model.EXPECT().AddExternalController(description.ExternalControllerArgs{
		Tag:    names.NewControllerTag("local-ctrl-uuid"),
		Addrs:  []string{"10.0.0.1:17070"},
		Alias:  "my-controller",
		CACert: "local-ca-cert",
		Models: []string{"uuid-2", "uuid-3"},
	}).Return(externalController)

	migration := ExportExternalControllers{}
	err := migration.Execute(source, model)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *ExternalControllersExportSuite) TestExportExternalControllerMixOfLocalAndExternal(c *gc.C) {
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
		expect.ID().Return("f47ac10b-58cc-4372-a567-0e02b2c3d479").Times(2)
		expect.Addrs().Return([]string{"10.0.0.1/24"})
		expect.Alias().Return("magic")
		expect.CACert().Return("magic-ca-cert")
		expect.Models().Return([]string{"uuid-2"})
	})

	localCtrl := s.migrationExternalController(ctrl, func(expect *MockMigrationExternalControllerMockRecorder) {
		expect.ID().Return("local-ctrl-uuid").Times(2)
		expect.Addrs().Return([]string{"10.0.0.2:17070"})
		expect.Alias().Return("my-controller")
		expect.CACert().Return("local-ca-cert")
		expect.Models().Return([]string{"uuid-3"})
	})

	externalController := NewMockExternalController(ctrl)

	source := NewMockExternalControllerSource(ctrl)
	source.EXPECT().AllRemoteApplications().Return(entities, nil)
	source.EXPECT().ControllerForModel("uuid-2").Return(extCtrlModel, nil)
	source.EXPECT().ControllerForModel("uuid-3").Return(nil, errors.NotFoundf("not found"))
	source.EXPECT().ModelExists("uuid-3").Return(true, nil)
	source.EXPECT().LocalControllerInfo([]string{"uuid-3"}).Return(localCtrl, nil)

	model := NewMockExternalControllerModel(ctrl)
	model.EXPECT().AddExternalController(description.ExternalControllerArgs{
		Tag:    names.NewControllerTag("f47ac10b-58cc-4372-a567-0e02b2c3d479"),
		Addrs:  []string{"10.0.0.1/24"},
		Alias:  "magic",
		CACert: "magic-ca-cert",
		Models: []string{"uuid-2"},
	}).Return(externalController)
	model.EXPECT().AddExternalController(description.ExternalControllerArgs{
		Tag:    names.NewControllerTag("local-ctrl-uuid"),
		Addrs:  []string{"10.0.0.2:17070"},
		Alias:  "my-controller",
		CACert: "local-ca-cert",
		Models: []string{"uuid-3"},
	}).Return(externalController)

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
