// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelmigration

import (
	"testing"

	"github.com/juju/description/v9"
	"github.com/juju/tc"
	gomock "go.uber.org/mock/gomock"

	"github.com/juju/juju/domain/agentpassword"
	"github.com/juju/juju/internal/testhelpers"
)

type exportSuite struct {
	testhelpers.IsolationSuite

	exportService *MockExportService
}

func TestExportSuite(t *testing.T) {
	tc.Run(t, &exportSuite{})
}

func (s *exportSuite) TestExportUnitPasswordHashes(c *tc.C) {
	defer s.setupMocks(c).Finish()

	hashes := agentpassword.UnitPasswordHashes{
		"foo/0": "hash",
	}

	s.exportService.EXPECT().GetAllUnitPasswordHashes(gomock.Any()).Return(hashes, nil)
	s.exportService.EXPECT().GetAllMachinePasswordHashes(gomock.Any()).Return(nil, nil)

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

	err := op.Execute(c.Context(), model)
	c.Assert(err, tc.ErrorIsNil)

	c.Check(unit.PasswordHash(), tc.Equals, "hash")
}

func (s *exportSuite) TestExportUnitPasswordHashesNoPasswords(c *tc.C) {
	defer s.setupMocks(c).Finish()

	hashes := agentpassword.UnitPasswordHashes{}

	s.exportService.EXPECT().GetAllUnitPasswordHashes(gomock.Any()).Return(hashes, nil)
	s.exportService.EXPECT().GetAllMachinePasswordHashes(gomock.Any()).Return(nil, nil)

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

	err := op.Execute(c.Context(), model)
	c.Assert(err, tc.ErrorIsNil)

	c.Check(unit.PasswordHash(), tc.Equals, "")
}

func (s *exportSuite) TestExportUnitPasswordHashesNoPasswordForUnit(c *tc.C) {
	defer s.setupMocks(c).Finish()

	hashes := agentpassword.UnitPasswordHashes{
		"foo/1": "hash",
	}

	s.exportService.EXPECT().GetAllUnitPasswordHashes(gomock.Any()).Return(hashes, nil)
	s.exportService.EXPECT().GetAllMachinePasswordHashes(gomock.Any()).Return(nil, nil)

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

	err := op.Execute(c.Context(), model)
	c.Assert(err, tc.ErrorIsNil)

	c.Check(unit.PasswordHash(), tc.Equals, "")
}

func (s *exportSuite) TestExportMachinePasswordHashes(c *tc.C) {
	defer s.setupMocks(c).Finish()

	hashes := agentpassword.MachinePasswordHashes{
		"0": "hash",
	}

	s.exportService.EXPECT().GetAllUnitPasswordHashes(gomock.Any()).Return(nil, nil)
	s.exportService.EXPECT().GetAllMachinePasswordHashes(gomock.Any()).Return(hashes, nil)

	op := exportOperation{
		service: s.exportService,
	}

	model := description.NewModel(description.ModelArgs{})
	machine := model.AddMachine(description.MachineArgs{
		Id: "0",
	})

	err := op.Execute(c.Context(), model)
	c.Assert(err, tc.ErrorIsNil)

	c.Check(machine.PasswordHash(), tc.Equals, "hash")
}

func (s *exportSuite) TestExportMachinePasswordHashesNoPasswords(c *tc.C) {
	defer s.setupMocks(c).Finish()

	hashes := agentpassword.MachinePasswordHashes{}

	s.exportService.EXPECT().GetAllUnitPasswordHashes(gomock.Any()).Return(nil, nil)
	s.exportService.EXPECT().GetAllMachinePasswordHashes(gomock.Any()).Return(hashes, nil)

	op := exportOperation{
		service: s.exportService,
	}

	model := description.NewModel(description.ModelArgs{})
	machine := model.AddMachine(description.MachineArgs{
		Id: "0",
	})

	err := op.Execute(c.Context(), model)
	c.Assert(err, tc.ErrorIsNil)

	c.Check(machine.PasswordHash(), tc.Equals, "")
}

func (s *exportSuite) TestExportMachinePasswordHashesNoPasswordForMachine(c *tc.C) {
	defer s.setupMocks(c).Finish()

	hashes := agentpassword.MachinePasswordHashes{
		"1": "hash",
	}

	s.exportService.EXPECT().GetAllUnitPasswordHashes(gomock.Any()).Return(nil, nil)
	s.exportService.EXPECT().GetAllMachinePasswordHashes(gomock.Any()).Return(hashes, nil)

	op := exportOperation{
		service: s.exportService,
	}

	model := description.NewModel(description.ModelArgs{})
	machine := model.AddMachine(description.MachineArgs{
		Id: "0",
	})

	err := op.Execute(c.Context(), model)
	c.Assert(err, tc.ErrorIsNil)

	c.Check(machine.PasswordHash(), tc.Equals, "")
}

func (s *exportSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.exportService = NewMockExportService(ctrl)

	return ctrl
}
