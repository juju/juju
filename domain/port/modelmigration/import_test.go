// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelmigration

import (
	"context"

	"github.com/juju/description/v9"
	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/core/network"
	coreunit "github.com/juju/juju/core/unit"
	coreunittesting "github.com/juju/juju/core/unit/testing"
	porterrors "github.com/juju/juju/domain/port/errors"
	loggertesting "github.com/juju/juju/internal/logger/testing"
)

type importSuite struct {
	coordinator *MockCoordinator
	portService *MockPortService
}

var _ = tc.Suite(&importSuite{})

func (s *importSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.coordinator = NewMockCoordinator(ctrl)
	s.portService = NewMockPortService(ctrl)

	return ctrl
}

func (s *importSuite) newImportOperation() *importOperation {
	return &importOperation{
		portService: s.portService,
	}
}

func (s *importSuite) TestRegisterImport(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.coordinator.EXPECT().Add(gomock.Any())

	RegisterImport(s.coordinator, loggertesting.WrapCheckLog(c))
}

func (s *importSuite) TestNoModelUserPermissions(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Empty model.
	model := description.NewModel(description.ModelArgs{})

	op := s.newImportOperation()
	err := op.Execute(context.Background(), model)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *importSuite) TestImport(c *tc.C) {
	defer s.setupMocks(c).Finish()

	model := description.NewModel(description.ModelArgs{})
	app := model.AddApplication(description.ApplicationArgs{
		Name: "app",
	})
	app.AddOpenedPortRange(description.OpenedPortRangeArgs{
		UnitName:     "unit/1",
		EndpointName: "endpoint-1",
		FromPort:     100,
		ToPort:       200,
		Protocol:     "udp",
	})
	machine := model.AddMachine(description.MachineArgs{
		Id: "1",
	})
	machine.AddOpenedPortRange(description.OpenedPortRangeArgs{
		UnitName:     "unit/2",
		EndpointName: "endpoint-2",
		FromPort:     300,
		ToPort:       400,
		Protocol:     "tcp",
	})

	unit1UUID := coreunittesting.GenUnitUUID(c)
	s.portService.EXPECT().GetUnitUUID(gomock.Any(), coreunit.Name("unit/1")).Return(unit1UUID, nil)
	s.portService.EXPECT().UpdateUnitPorts(gomock.Any(), unit1UUID, network.GroupedPortRanges{
		"endpoint-1": []network.PortRange{{
			FromPort: 100,
			ToPort:   200,
			Protocol: "udp",
		}},
	}, nil)

	unit2UUID := coreunittesting.GenUnitUUID(c)
	s.portService.EXPECT().GetUnitUUID(gomock.Any(), coreunit.Name("unit/2")).Return(unit2UUID, nil)
	s.portService.EXPECT().UpdateUnitPorts(gomock.Any(), unit2UUID, network.GroupedPortRanges{
		"endpoint-2": []network.PortRange{{
			FromPort: 300,
			ToPort:   400,
			Protocol: "tcp",
		}},
	}, nil)

	op := s.newImportOperation()
	err := op.Execute(context.Background(), model)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *importSuite) TestImportUnitNotFound(c *tc.C) {
	defer s.setupMocks(c).Finish()

	model := description.NewModel(description.ModelArgs{})
	app := model.AddApplication(description.ApplicationArgs{
		Name: "app",
	})
	app.AddOpenedPortRange(description.OpenedPortRangeArgs{
		UnitName:     "unit/1",
		EndpointName: "endpoint-1",
		FromPort:     100,
		ToPort:       200,
		Protocol:     "udp",
	})

	s.portService.EXPECT().GetUnitUUID(gomock.Any(), coreunit.Name("unit/1")).Return("", porterrors.UnitNotFound)

	op := s.newImportOperation()
	err := op.Execute(context.Background(), model)
	c.Assert(err, tc.ErrorIs, porterrors.UnitNotFound)
}
