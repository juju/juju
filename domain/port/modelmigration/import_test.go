// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelmigration

import (
	"context"

	"github.com/juju/description/v8"
	"github.com/juju/names/v5"
	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/network"
	uniterrors "github.com/juju/juju/domain/unitstate/errors"
	loggertesting "github.com/juju/juju/internal/logger/testing"
)

type importSuite struct {
	coordinator *MockCoordinator
	service     *MockImportService
}

var _ = gc.Suite(&importSuite{})

func (s *importSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.coordinator = NewMockCoordinator(ctrl)
	s.service = NewMockImportService(ctrl)

	return ctrl
}

func (s *importSuite) newImportOperation() *importOperation {
	return &importOperation{
		service: s.service,
	}
}

func (s *importSuite) TestRegisterImport(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.coordinator.EXPECT().Add(gomock.Any())

	RegisterImport(s.coordinator, loggertesting.WrapCheckLog(c))
}

func (s *importSuite) TestNoModelUserPermissions(c *gc.C) {
	defer s.setupMocks(c).Finish()

	// Empty model.
	model := description.NewModel(description.ModelArgs{})

	op := s.newImportOperation()
	err := op.Execute(context.Background(), model)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *importSuite) TestImport(c *gc.C) {
	defer s.setupMocks(c).Finish()

	model := description.NewModel(description.ModelArgs{})
	app := model.AddApplication(description.ApplicationArgs{
		Tag: names.NewApplicationTag("app"),
	})
	app.AddOpenedPortRange(description.OpenedPortRangeArgs{
		UnitName:     "unit-1",
		EndpointName: "endpoint-1",
		FromPort:     100,
		ToPort:       200,
		Protocol:     "udp",
	})
	machine := model.AddMachine(description.MachineArgs{
		Id: names.NewMachineTag("1"),
	})
	machine.AddOpenedPortRange(description.OpenedPortRangeArgs{
		UnitName:     "unit-2",
		EndpointName: "endpoint-2",
		FromPort:     300,
		ToPort:       400,
		Protocol:     "tcp",
	})

	s.service.EXPECT().SetUnitPorts(gomock.Any(), "unit-1", network.GroupedPortRanges{
		"endpoint-1": []network.PortRange{{
			FromPort: 100,
			ToPort:   200,
			Protocol: "udp",
		}},
	})

	s.service.EXPECT().SetUnitPorts(gomock.Any(), "unit-2", network.GroupedPortRanges{
		"endpoint-2": []network.PortRange{{
			FromPort: 300,
			ToPort:   400,
			Protocol: "tcp",
		}},
	})

	op := s.newImportOperation()
	err := op.Execute(context.Background(), model)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *importSuite) TestImportUnitNotFound(c *gc.C) {
	defer s.setupMocks(c).Finish()

	model := description.NewModel(description.ModelArgs{})
	app := model.AddApplication(description.ApplicationArgs{
		Tag: names.NewApplicationTag("app"),
	})
	app.AddOpenedPortRange(description.OpenedPortRangeArgs{
		UnitName:     "unit-1",
		EndpointName: "endpoint-1",
		FromPort:     100,
		ToPort:       200,
		Protocol:     "udp",
	})

	s.service.EXPECT().SetUnitPorts(gomock.Any(), "unit-1", network.GroupedPortRanges{
		"endpoint-1": []network.PortRange{{
			FromPort: 100,
			ToPort:   200,
			Protocol: "udp",
		}},
	}).Return(uniterrors.UnitNotFound)

	op := s.newImportOperation()
	err := op.Execute(context.Background(), model)
	c.Assert(err, jc.ErrorIs, uniterrors.UnitNotFound)
}
