// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelmigration

import (
	"testing"

	"github.com/juju/description/v11"
	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	coreagentbinary "github.com/juju/juju/core/agentbinary"
	coremachine "github.com/juju/juju/core/machine"
	"github.com/juju/juju/core/semversion"
	coreunit "github.com/juju/juju/core/unit"
	loggertesting "github.com/juju/juju/internal/logger/testing"
)

type importModelAgentSuite struct {
	importService *MockImportService
}

func TestImportModelAgentSuite(t *testing.T) {
	tc.Run(t, &importModelAgentSuite{})
}

func (s *importModelAgentSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.importService = NewMockImportService(ctrl)

	c.Cleanup(func() {
		s.importService = nil
	})

	return ctrl
}

func (s *importModelAgentSuite) newUnitImportOperation(c *tc.C) *importUnitAgentBinaryOperation {
	bOp := baseAgentBinaryImportOperation{
		importService: s.importService,
		logger:        loggertesting.WrapCheckLog(c),
	}
	return &importUnitAgentBinaryOperation{
		baseAgentBinaryImportOperation: bOp,
	}
}

func (s *importModelAgentSuite) TestImportUnitAgentBinaryOperation(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	// Arrange: model description with a unit having tools
	model := description.NewModel(description.ModelArgs{})
	app := model.AddApplication(description.ApplicationArgs{
		Name: "app-test",
	})
	unit := app.AddUnit(description.UnitArgs{
		Name: "app-test/1",
	})
	unit.SetTools(description.AgentToolsArgs{
		Version: "3.8.42-ubuntu-amd64",
	})

	// Arrange: add a unit with no tools
	app.AddUnit(description.UnitArgs{
		Name: "app-test/2",
	})

	// Arrange: import operation
	s.importService.EXPECT().SetUnitReportedAgentVersion(gomock.Any(),
		coreunit.Name("app-test/1"),
		coreagentbinary.Version{
			Number: semversion.Number{
				Major: 3,
				Minor: 8,
				Patch: 42,
			},
			Arch: "amd64",
		})
	op := s.newUnitImportOperation(c)

	// Act: execute import operation
	err := op.Execute(c.Context(), model)

	// Assert: no error
	c.Assert(err, tc.ErrorIsNil)
}

func (s *importModelAgentSuite) newMachineImportOperation(c *tc.C) *importMachineAgentBinaryOperation {
	bOp := baseAgentBinaryImportOperation{
		importService: s.importService,
		logger:        loggertesting.WrapCheckLog(c),
	}
	return &importMachineAgentBinaryOperation{
		baseAgentBinaryImportOperation: bOp,
	}
}

func (s *importModelAgentSuite) TestImportMachineAgentBinaryOperation(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	// Arrange: model description with a machine having tools
	model := description.NewModel(description.ModelArgs{})
	machine := model.AddMachine(description.MachineArgs{
		Id: "42",
	})
	machine.SetTools(description.AgentToolsArgs{
		Version: "3.8.42-ubuntu-amd64",
	})

	// Arrange: import operation
	s.importService.EXPECT().SetMachineReportedAgentVersion(gomock.Any(),
		coremachine.Name("42"),
		coreagentbinary.Version{
			Number: semversion.Number{
				Major: 3,
				Minor: 8,
				Patch: 42,
			},
			Arch: "amd64",
		})
	op := s.newMachineImportOperation(c)

	// Act: execute import operation
	err := op.Execute(c.Context(), model)

	// Assert: no error
	c.Assert(err, tc.ErrorIsNil)
}
