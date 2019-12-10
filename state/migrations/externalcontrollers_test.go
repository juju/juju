// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package migrations

import (
	gomock "github.com/golang/mock/gomock"
	description "github.com/juju/description"
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
)

type ExternalControllersExportSuite struct{}

var _ = gc.Suite(&ExternalControllersExportSuite{})

func (s *ExternalControllersExportSuite) TestExportExternalController(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	entities := []MigrationExternalController{
		s.migrationExternalController(ctrl, func(expect *MockMigrationExternalControllerMockRecorder) {
			expect.Addrs().Return([]string{"10.0.0.1/24"})
			expect.Alias().Return("magic")
			expect.CACert().Return("magic-ca-cert")
		}),
	}

	externalController := NewMockExternalController(ctrl)

	source := NewMockExternalControllerSource(ctrl)
	source.EXPECT().AllExternalControllers().Return(entities, nil)

	model := NewMockExternalControllerModel(ctrl)
	model.EXPECT().AddExternalController(description.ExternalControllerArgs{
		Addrs:  []string{"10.0.0.1/24"},
		Alias:  "magic",
		CACert: "magic-ca-cert",
	}).Return(externalController)

	migration := ExportExternalControllers{}
	err := migration.Execute(source, model)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *ExternalControllersExportSuite) TestExportExternalControllerFailsGettingEntities(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	entities := []MigrationExternalController{
		NewMockMigrationExternalController(ctrl),
	}

	source := NewMockExternalControllerSource(ctrl)
	source.EXPECT().AllExternalControllers().Return(entities, errors.New("fail"))

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
