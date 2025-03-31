// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelmigration

import (
	"context"

	"github.com/juju/description/v9"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gomock "go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/unit"
	"github.com/juju/juju/domain/passwords"
	"github.com/juju/juju/internal/errors"
)

type importSuite struct {
	testing.IsolationSuite

	importService *MockImportService
}

var _ = gc.Suite(&importSuite{})

func (s *importSuite) TestImportUnitPasswordHash(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.importService.EXPECT().SetUnitPasswordHash(gomock.Any(), unit.Name("foo/0"), passwords.PasswordHash("hash")).Return(nil)

	op := importOperation{
		service: s.importService,
	}

	model := description.NewModel(description.ModelArgs{})
	application := model.AddApplication(description.ApplicationArgs{
		Name: "foo",
	})
	application.AddUnit(description.UnitArgs{
		Name:         "foo/0",
		PasswordHash: "hash",
	})

	err := op.Execute(context.Background(), model)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *importSuite) TestImportUnitPasswordHashError(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.importService.EXPECT().SetUnitPasswordHash(gomock.Any(), unit.Name("foo/0"), passwords.PasswordHash("hash")).Return(errors.Errorf("boom"))

	op := importOperation{
		service: s.importService,
	}

	model := description.NewModel(description.ModelArgs{})
	application := model.AddApplication(description.ApplicationArgs{
		Name: "foo",
	})
	application.AddUnit(description.UnitArgs{
		Name:         "foo/0",
		PasswordHash: "hash",
	})

	err := op.Execute(context.Background(), model)
	c.Assert(err, gc.ErrorMatches, ".*boom")
}

func (s *importSuite) TestImportUnitPasswordHashMissingHash(c *gc.C) {
	defer s.setupMocks(c).Finish()

	op := importOperation{
		service: s.importService,
	}

	model := description.NewModel(description.ModelArgs{})
	application := model.AddApplication(description.ApplicationArgs{
		Name: "foo",
	})
	application.AddUnit(description.UnitArgs{
		Name: "foo/0",
	})

	err := op.Execute(context.Background(), model)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *importSuite) TestImportUnitPasswordHashNoApplications(c *gc.C) {
	defer s.setupMocks(c).Finish()

	op := importOperation{
		service: s.importService,
	}

	model := description.NewModel(description.ModelArgs{})

	err := op.Execute(context.Background(), model)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *importSuite) TestImportUnitPasswordHashNoUnits(c *gc.C) {
	defer s.setupMocks(c).Finish()

	op := importOperation{
		service: s.importService,
	}

	model := description.NewModel(description.ModelArgs{})
	model.AddApplication(description.ApplicationArgs{
		Name: "foo",
	})

	err := op.Execute(context.Background(), model)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *importSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.importService = NewMockImportService(ctrl)

	return ctrl
}
