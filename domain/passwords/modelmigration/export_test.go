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

	unit "github.com/juju/juju/core/unit"
	"github.com/juju/juju/domain/passwords"
)

type exportSuite struct {
	testing.IsolationSuite

	exportService *MockExportService
}

var _ = gc.Suite(&exportSuite{})

func (s *exportSuite) TestExportUnitPasswordHashes(c *gc.C) {
	defer s.setupMocks(c).Finish()

	hashes := map[string]map[unit.Name]passwords.PasswordHash{
		"foo": {
			"foo/0": "hash",
		},
	}

	s.exportService.EXPECT().GetAllUnitPasswordHashes(gomock.Any()).Return(hashes, nil)

	op := exportOperation{
		service: s.exportService,
	}

	model := description.NewModel(description.ModelArgs{})
	application := model.AddApplication(description.ApplicationArgs{
		Name: "foo",
	})
	unit := application.AddUnit(description.UnitArgs{
		Name: "foo/0",
	})

	err := op.Execute(context.Background(), model)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(unit.PasswordHash(), gc.Equals, "hash")
}

func (s *exportSuite) TestExportUnitPasswordHashesNoPasswords(c *gc.C) {
	defer s.setupMocks(c).Finish()

	hashes := map[string]map[unit.Name]passwords.PasswordHash{}

	s.exportService.EXPECT().GetAllUnitPasswordHashes(gomock.Any()).Return(hashes, nil)

	op := exportOperation{
		service: s.exportService,
	}

	model := description.NewModel(description.ModelArgs{})
	application := model.AddApplication(description.ApplicationArgs{
		Name: "foo",
	})
	unit := application.AddUnit(description.UnitArgs{
		Name: "foo/0",
	})

	err := op.Execute(context.Background(), model)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(unit.PasswordHash(), gc.Equals, "")
}

func (s *exportSuite) TestExportUnitPasswordHashesNoPasswordForUnit(c *gc.C) {
	defer s.setupMocks(c).Finish()

	hashes := map[string]map[unit.Name]passwords.PasswordHash{
		"foo": {
			"foo/1": "hash",
		},
	}

	s.exportService.EXPECT().GetAllUnitPasswordHashes(gomock.Any()).Return(hashes, nil)

	op := exportOperation{
		service: s.exportService,
	}

	model := description.NewModel(description.ModelArgs{})
	application := model.AddApplication(description.ApplicationArgs{
		Name: "foo",
	})
	unit := application.AddUnit(description.UnitArgs{
		Name: "foo/0",
	})

	err := op.Execute(context.Background(), model)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(unit.PasswordHash(), gc.Equals, "")
}

func (s *exportSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.exportService = NewMockExportService(ctrl)

	return ctrl
}
