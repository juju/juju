// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"database/sql"
	"testing"

	"github.com/canonical/sqlair"
	"github.com/juju/clock"
	"github.com/juju/tc"

	coreapplication "github.com/juju/juju/core/application"
	"github.com/juju/juju/core/arch"
	"github.com/juju/juju/core/machine"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/relation"
	"github.com/juju/juju/domain/application"
	"github.com/juju/juju/domain/application/architecture"
	"github.com/juju/juju/domain/application/charm"
	applicationstate "github.com/juju/juju/domain/application/state"
	"github.com/juju/juju/domain/constraints"
	"github.com/juju/juju/domain/deployment"
	domainmachine "github.com/juju/juju/domain/machine"
	machinestate "github.com/juju/juju/domain/machine/state"
	domainnetwork "github.com/juju/juju/domain/network"
	porterrors "github.com/juju/juju/domain/port/errors"
	"github.com/juju/juju/internal/errors"
	"github.com/juju/juju/internal/logger"
	loggertesting "github.com/juju/juju/internal/logger/testing"
)

type updateUnitPortsSuite struct {
	baseSuite

	appUUID   coreapplication.UUID
	unitCount int
}

func TestUpdateUnitPortsSuite(t *testing.T) {
	tc.Run(t, &updateUnitPortsSuite{})
}

var (
	machineUUIDs []string
	netNodeUUIDs []string
	appNames     = []string{"app-zero", "app-one"}
)

func (s *updateUnitPortsSuite) SetUpTest(c *tc.C) {
	s.baseSuite.SetUpTest(c)

	machineSt := machinestate.NewState(s.TxnRunnerFactory(), clock.WallClock, logger.GetLogger("juju.test.machine"))

	netNodeUUID0, machineNames0, err := machineSt.AddMachine(c.Context(), domainmachine.AddMachineArgs{
		Platform: deployment.Platform{
			Channel: "24.04",
			OSType:  deployment.Ubuntu,
		},
	})
	c.Assert(err, tc.ErrorIsNil)
	machineUUID0, err := machineSt.GetMachineUUID(c.Context(), machineNames0[0])
	c.Assert(err, tc.ErrorIsNil)
	netNodeUUID1, machineNames1, err := machineSt.AddMachine(c.Context(), domainmachine.AddMachineArgs{
		Platform: deployment.Platform{
			Channel: "24.04",
			OSType:  deployment.Ubuntu,
		},
	})
	c.Assert(err, tc.ErrorIsNil)
	machineUUID1, err := machineSt.GetMachineUUID(c.Context(), machineNames1[0])
	c.Assert(err, tc.ErrorIsNil)

	machineUUIDs = []string{machineUUID0.String(), machineUUID1.String()}
	netNodeUUIDs = []string{netNodeUUID0, netNodeUUID1}

	s.appUUID = s.createApplicationWithRelations(c, appNames[0], "ep0", "ep1", "ep2")
	s.unitUUID, s.unitName = s.createUnit(c, netNodeUUIDs[0], appNames[0])

	c.Cleanup(func() {
		s.unitCount = 0
	})
}

func (s *updateUnitPortsSuite) initialiseOpenPort(c *tc.C) {
	s.runUpdateUnitPorts(c, s.unitUUID, network.GroupedPortRanges{
		"ep0": {
			{Protocol: "tcp", FromPort: 80, ToPort: 80},
			{Protocol: "udp", FromPort: 1000, ToPort: 1500},
		},
		"ep1": {
			{Protocol: "tcp", FromPort: 8080, ToPort: 8080},
		},
	}, network.GroupedPortRanges{})
}

func (s *updateUnitPortsSuite) runUpdateUnitPorts(c *tc.C, unitUUID string, open, close network.GroupedPortRanges) {
	ctx := c.Context()
	err := s.TxnRunner().Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		return s.state.updateUnitPorts(ctx, tx, unitUUID, open, close)
	})
	c.Assert(err, tc.ErrorIsNil)
}

func (s *updateUnitPortsSuite) runUpdateUnitPortsWithError(c *tc.C, unitUUID string, open, close network.GroupedPortRanges, expectedErr error) {
	ctx := c.Context()
	err := s.TxnRunner().Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		return s.state.updateUnitPorts(ctx, tx, unitUUID, open, close)
	})
	c.Assert(err, tc.ErrorIs, expectedErr)
}

func (s *updateUnitPortsSuite) TestGetColocatedOpenedPortsSingleUnit(c *tc.C) {
	ctx := c.Context()
	s.initialiseOpenPort(c)

	db, err := s.state.DB(ctx)
	c.Assert(err, tc.ErrorIsNil)

	var opendPorts []network.PortRange
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		var err error
		opendPorts, err = s.state.getColocatedOpenedPorts(ctx, tx, unitUUID{UUID: s.unitUUID})
		return err
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(opendPorts, tc.HasLen, 3)
	c.Check(opendPorts[0], tc.DeepEquals, network.PortRange{Protocol: "tcp", FromPort: 80, ToPort: 80})
	c.Check(opendPorts[1], tc.DeepEquals, network.PortRange{Protocol: "tcp", FromPort: 8080, ToPort: 8080})
	c.Check(opendPorts[2], tc.DeepEquals, network.PortRange{Protocol: "udp", FromPort: 1000, ToPort: 1500})
}

func (s *updateUnitPortsSuite) TestGetColocatedOpenedPortsMultipleUnits(c *tc.C) {
	ctx := c.Context()
	s.initialiseOpenPort(c)

	unit1UUID, _ := s.createUnit(c, netNodeUUIDs[0], appNames[0])
	s.runUpdateUnitPorts(c, unit1UUID, network.GroupedPortRanges{
		"ep0": {
			{Protocol: "tcp", FromPort: 443, ToPort: 443},
			{Protocol: "udp", FromPort: 2000, ToPort: 2500},
		},
	}, network.GroupedPortRanges{})

	db, err := s.state.DB(ctx)
	c.Assert(err, tc.ErrorIsNil)

	var opendPorts []network.PortRange
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		var err error
		opendPorts, err = s.state.getColocatedOpenedPorts(ctx, tx, unitUUID{UUID: s.unitUUID})
		return err
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(opendPorts, tc.HasLen, 5)
	c.Check(opendPorts[0], tc.DeepEquals, network.PortRange{Protocol: "tcp", FromPort: 80, ToPort: 80})
	c.Check(opendPorts[1], tc.DeepEquals, network.PortRange{Protocol: "tcp", FromPort: 443, ToPort: 443})
	c.Check(opendPorts[2], tc.DeepEquals, network.PortRange{Protocol: "tcp", FromPort: 8080, ToPort: 8080})
	c.Check(opendPorts[3], tc.DeepEquals, network.PortRange{Protocol: "udp", FromPort: 1000, ToPort: 1500})
	c.Check(opendPorts[4], tc.DeepEquals, network.PortRange{Protocol: "udp", FromPort: 2000, ToPort: 2500})
}

func (s *updateUnitPortsSuite) TestGetColocatedOpenedPortsMultipleUnitsOnNetNodes(c *tc.C) {
	ctx := c.Context()
	s.initialiseOpenPort(c)

	unit1UUID, _ := s.createUnit(c, netNodeUUIDs[1], appNames[0])
	s.runUpdateUnitPorts(c, unit1UUID, network.GroupedPortRanges{
		"ep0": {
			{Protocol: "tcp", FromPort: 443, ToPort: 443},
			{Protocol: "udp", FromPort: 2000, ToPort: 2500},
		},
	}, network.GroupedPortRanges{})

	db, err := s.state.DB(ctx)
	c.Assert(err, tc.ErrorIsNil)

	var opendPorts []network.PortRange
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		var err error
		opendPorts, err = s.state.getColocatedOpenedPorts(ctx, tx, unitUUID{UUID: s.unitUUID})
		return err
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(opendPorts, tc.HasLen, 3)
	c.Check(opendPorts[0], tc.DeepEquals, network.PortRange{Protocol: "tcp", FromPort: 80, ToPort: 80})
	c.Check(opendPorts[1], tc.DeepEquals, network.PortRange{Protocol: "tcp", FromPort: 8080, ToPort: 8080})
	c.Check(opendPorts[2], tc.DeepEquals, network.PortRange{Protocol: "udp", FromPort: 1000, ToPort: 1500})
}

func (s *updateUnitPortsSuite) TestGetWildcardEndpointOpenedPorts(c *tc.C) {
	ctx := c.Context()

	s.runUpdateUnitPorts(c, s.unitUUID, network.GroupedPortRanges{
		network.WildcardEndpoint: {network.MustParsePortRange("100-200/tcp")},
	}, network.GroupedPortRanges{})

	db, err := s.state.DB(ctx)
	c.Assert(err, tc.ErrorIsNil)

	var portRanges []network.PortRange
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		var err error
		portRanges, err = s.state.getWildcardEndpointOpenedPorts(ctx, tx, unitUUID{UUID: s.unitUUID})
		return err
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(portRanges, tc.HasLen, 1)
	c.Check(portRanges[0], tc.DeepEquals, network.MustParsePortRange("100-200/tcp"))
}

func (s *updateUnitPortsSuite) TestGetWildcardEndpointOpenedPortsIgnoresOtherEndpoints(c *tc.C) {
	ctx := c.Context()
	s.initialiseOpenPort(c)

	db, err := s.state.DB(ctx)
	c.Assert(err, tc.ErrorIsNil)

	var portRanges []network.PortRange
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		var err error
		portRanges, err = s.state.getWildcardEndpointOpenedPorts(ctx, tx, unitUUID{UUID: s.unitUUID})
		return err
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(portRanges, tc.HasLen, 0)
}

func (s *updateUnitPortsSuite) TestGetEndpointsForPopulatedUnit(c *tc.C) {
	ctx := c.Context()
	s.initialiseOpenPort(c)

	db, err := s.state.DB(ctx)
	c.Assert(err, tc.ErrorIsNil)

	var endpoints []string
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		var err error
		endpoints, err = s.state.getEndpoints(ctx, tx, unitUUID{UUID: s.unitUUID})
		return err
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(endpoints, tc.DeepEquals, []string{"ep0", "ep1", "ep2", relation.JujuInfo})
}

func (s *updateUnitPortsSuite) TestGetEndpointsForUnpopulatedUnit(c *tc.C) {
	ctx := c.Context()

	db, err := s.state.DB(ctx)
	c.Assert(err, tc.ErrorIsNil)

	var endpoints []string
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		var err error
		endpoints, err = s.state.getEndpoints(ctx, tx, unitUUID{UUID: s.unitUUID})
		return err
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(endpoints, tc.DeepEquals, []string{"ep0", "ep1", "ep2", relation.JujuInfo})
}

func (s *updateUnitPortsSuite) TestUpdateUnitPortsOpenPort(c *tc.C) {
	// Arrange
	s.initialiseOpenPort(c)

	// Act
	s.runUpdateUnitPorts(c, s.unitUUID, network.GroupedPortRanges{"ep0": {{Protocol: "tcp", FromPort: 1000, ToPort: 1500}}}, network.GroupedPortRanges{})

	// Assert
	obtainedPortRanges := s.getUnitOpenedPorts(c, s.unitUUID)
	c.Check(obtainedPortRanges, tc.SameContents, []endpointPortRange{
		{Endpoint: "ep0", FromPort: 80, ToPort: 80, Protocol: "tcp"},
		{Endpoint: "ep0", FromPort: 1000, ToPort: 1500, Protocol: "tcp"},
		{Endpoint: "ep0", FromPort: 1000, ToPort: 1500, Protocol: "udp"},
		{Endpoint: "ep1", FromPort: 8080, ToPort: 8080, Protocol: "tcp"},
	})
}

func (s *updateUnitPortsSuite) TestUpdateUnitPortsOpenPortWildcardEndpoint(c *tc.C) {
	// Act
	s.runUpdateUnitPorts(c, s.unitUUID, network.GroupedPortRanges{
		network.WildcardEndpoint: {{Protocol: "tcp", FromPort: 1000, ToPort: 1500}},
	}, network.GroupedPortRanges{})

	// Assert
	obtainedPortRanges := s.getUnitOpenedPorts(c, s.unitUUID)
	c.Check(obtainedPortRanges, tc.SameContents, []endpointPortRange{
		{Endpoint: network.WildcardEndpoint, FromPort: 1000, ToPort: 1500, Protocol: "tcp"},
	})
}

func (s *updateUnitPortsSuite) TestUpdateUnitPortsOpenOnInvalidEndpoint(c *tc.C) {
	s.runUpdateUnitPortsWithError(c, s.unitUUID, network.GroupedPortRanges{
		"invalid": {{Protocol: "tcp", FromPort: 1000, ToPort: 1500}},
	}, network.GroupedPortRanges{}, porterrors.InvalidEndpoint)
}

func (s *updateUnitPortsSuite) TestUpdateUnitPortsClosePort(c *tc.C) {
	// Arrange
	s.initialiseOpenPort(c)

	obtainedPortRanges := s.getUnitOpenedPorts(c, s.unitUUID)
	c.Check(obtainedPortRanges, tc.SameContents, []endpointPortRange{
		{Endpoint: "ep0", FromPort: 80, ToPort: 80, Protocol: "tcp"},
		{Endpoint: "ep0", FromPort: 1000, ToPort: 1500, Protocol: "udp"},
		{Endpoint: "ep1", FromPort: 8080, ToPort: 8080, Protocol: "tcp"},
	})

	// Act
	s.runUpdateUnitPorts(c, s.unitUUID, network.GroupedPortRanges{}, network.GroupedPortRanges{"ep0": {{Protocol: "tcp", FromPort: 80, ToPort: 80}}})

	// Assert
	obtainedPortRanges = s.getUnitOpenedPorts(c, s.unitUUID)
	c.Check(obtainedPortRanges, tc.SameContents, []endpointPortRange{
		{Endpoint: "ep0", FromPort: 1000, ToPort: 1500, Protocol: "udp"},
		{Endpoint: "ep1", FromPort: 8080, ToPort: 8080, Protocol: "tcp"},
	})
}

func (s *updateUnitPortsSuite) TestUpdateUnitPortsOpenPortRangeAdjacent(c *tc.C) {
	// Arrange
	s.initialiseOpenPort(c)

	// Act
	s.runUpdateUnitPorts(c, s.unitUUID, network.GroupedPortRanges{"ep0": {{Protocol: "udp", FromPort: 1501, ToPort: 2000}}}, network.GroupedPortRanges{})

	// Assert
	obtainedPortRanges := s.getUnitOpenedPorts(c, s.unitUUID)
	c.Check(obtainedPortRanges, tc.SameContents, []endpointPortRange{
		{Endpoint: "ep0", FromPort: 80, ToPort: 80, Protocol: "tcp"},
		{Endpoint: "ep0", FromPort: 1000, ToPort: 1500, Protocol: "udp"},
		{Endpoint: "ep0", FromPort: 1501, ToPort: 2000, Protocol: "udp"},
		{Endpoint: "ep1", FromPort: 8080, ToPort: 8080, Protocol: "tcp"},
	})
}

func (s *updateUnitPortsSuite) TestUpdateUnitPortsClosePortRange(c *tc.C) {
	// Arrange
	s.initialiseOpenPort(c)

	// Act
	s.runUpdateUnitPorts(c, s.unitUUID, network.GroupedPortRanges{}, network.GroupedPortRanges{"ep0": {{Protocol: "udp", FromPort: 1000, ToPort: 1500}}})

	// Assert
	obtainedPortRanges := s.getUnitOpenedPorts(c, s.unitUUID)
	c.Check(obtainedPortRanges, tc.SameContents, []endpointPortRange{
		{Endpoint: "ep0", FromPort: 80, ToPort: 80, Protocol: "tcp"},
		{Endpoint: "ep1", FromPort: 8080, ToPort: 8080, Protocol: "tcp"},
	})
}

func (s *updateUnitPortsSuite) TestUpdateUnitPortsClosePortEndpoint(c *tc.C) {
	// Arrange
	s.initialiseOpenPort(c)

	// Act
	s.runUpdateUnitPorts(c, s.unitUUID, network.GroupedPortRanges{}, network.GroupedPortRanges{
		"ep0": {
			{Protocol: "tcp", FromPort: 80, ToPort: 80},
			{Protocol: "udp", FromPort: 1000, ToPort: 1500},
		},
	})

	// Assert
	obtainedPortRanges := s.getUnitOpenedPorts(c, s.unitUUID)
	c.Check(obtainedPortRanges, tc.SameContents, []endpointPortRange{
		{Endpoint: "ep1", FromPort: 8080, ToPort: 8080, Protocol: "tcp"},
	})
}

func (s *updateUnitPortsSuite) TestUpdateUnitPortsOpenCloseICMP(c *tc.C) {
	// Arrange
	s.initialiseOpenPort(c)

	// Act
	s.runUpdateUnitPorts(c, s.unitUUID, network.GroupedPortRanges{"ep0": {{Protocol: "icmp"}}}, network.GroupedPortRanges{})

	// Assert
	obtainedPortRanges := s.getUnitOpenedPorts(c, s.unitUUID)
	c.Check(obtainedPortRanges, tc.SameContents, []endpointPortRange{
		{Endpoint: "ep0", Protocol: "icmp"},
		{Endpoint: "ep0", FromPort: 80, ToPort: 80, Protocol: "tcp"},
		{Endpoint: "ep0", FromPort: 1000, ToPort: 1500, Protocol: "udp"},
		{Endpoint: "ep1", FromPort: 8080, ToPort: 8080, Protocol: "tcp"},
	})

	// Act
	s.runUpdateUnitPorts(c, s.unitUUID, network.GroupedPortRanges{}, network.GroupedPortRanges{"ep0": {{Protocol: "icmp"}}})

	// Assert
	obtainedPortRanges = s.getUnitOpenedPorts(c, s.unitUUID)
	c.Check(obtainedPortRanges, tc.SameContents, []endpointPortRange{
		{Endpoint: "ep0", FromPort: 80, ToPort: 80, Protocol: "tcp"},
		{Endpoint: "ep0", FromPort: 1000, ToPort: 1500, Protocol: "udp"},
		{Endpoint: "ep1", FromPort: 8080, ToPort: 8080, Protocol: "tcp"},
	})
}

func (s *updateUnitPortsSuite) TestUpdateUnitPortsOpenPortRangeMixedEndpoints(c *tc.C) {
	// Arrange
	s.initialiseOpenPort(c)

	// Act
	s.runUpdateUnitPorts(c, s.unitUUID, network.GroupedPortRanges{
		"ep0": {{Protocol: "udp", FromPort: 2500, ToPort: 3000}},
		"ep2": {{Protocol: "udp", FromPort: 2000, ToPort: 2100}},
	}, network.GroupedPortRanges{})

	// Assert
	obtainedPortRanges := s.getUnitOpenedPorts(c, s.unitUUID)
	c.Check(obtainedPortRanges, tc.SameContents, []endpointPortRange{
		{Endpoint: "ep0", FromPort: 80, ToPort: 80, Protocol: "tcp"},
		{Endpoint: "ep0", FromPort: 1000, ToPort: 1500, Protocol: "udp"},
		{Endpoint: "ep0", FromPort: 2500, ToPort: 3000, Protocol: "udp"},
		{Endpoint: "ep1", FromPort: 8080, ToPort: 8080, Protocol: "tcp"},
		{Endpoint: "ep2", FromPort: 2000, ToPort: 2100, Protocol: "udp"},
	})
}

func (s *updateUnitPortsSuite) TestUpdateUnitPortsClosePortRangeMixedEndpoints(c *tc.C) {
	// Arrange
	s.initialiseOpenPort(c)

	s.runUpdateUnitPorts(c, s.unitUUID, network.GroupedPortRanges{
		"ep2": {
			{Protocol: "udp", FromPort: 2000, ToPort: 2500},
			{Protocol: "udp", FromPort: 3000, ToPort: 3000},
		},
	}, network.GroupedPortRanges{})

	// Act
	s.runUpdateUnitPorts(c, s.unitUUID, network.GroupedPortRanges{}, network.GroupedPortRanges{
		"ep0": {{Protocol: "udp", FromPort: 1000, ToPort: 1500}},
		"ep2": {{Protocol: "udp", FromPort: 2000, ToPort: 2500}},
	})

	// Assert
	obtainedPortRanges := s.getUnitOpenedPorts(c, s.unitUUID)
	c.Check(obtainedPortRanges, tc.SameContents, []endpointPortRange{
		{Endpoint: "ep0", FromPort: 80, ToPort: 80, Protocol: "tcp"},
		{Endpoint: "ep1", FromPort: 8080, ToPort: 8080, Protocol: "tcp"},
		{Endpoint: "ep2", FromPort: 3000, ToPort: 3000, Protocol: "udp"},
	})
}

func (s *updateUnitPortsSuite) TestUpdateUnitPortRangesOpenAlreadyOpenAcrossUnits(c *tc.C) {
	// Arrange
	s.initialiseOpenPort(c)
	unit1UUID, unit1Name := s.createUnit(c, netNodeUUIDs[0], appNames[0])

	// Act
	s.runUpdateUnitPorts(c, s.unitUUID, network.GroupedPortRanges{"ep0": {{Protocol: "udp", FromPort: 1000, ToPort: 1500}}}, network.GroupedPortRanges{})

	// Act: idempotent
	s.runUpdateUnitPorts(c, unit1UUID, network.GroupedPortRanges{"ep0": {{Protocol: "udp", FromPort: 1000, ToPort: 1500}}}, network.GroupedPortRanges{})

	// Assert
	obtainedOpenPorts := s.getMachineOpenedPorts(c, machineUUIDs[0])
	c.Check(obtainedOpenPorts, tc.SameContents, []unitEndpointPortRange{
		{UnitName: s.unitName, Endpoint: "ep0", FromPort: 80, ToPort: 80, Protocol: "tcp"},
		{UnitName: s.unitName, Endpoint: "ep1", FromPort: 8080, ToPort: 8080, Protocol: "tcp"},
		{UnitName: s.unitName, Endpoint: "ep0", FromPort: 1000, ToPort: 1500, Protocol: "udp"},
		{UnitName: unit1Name, Endpoint: "ep0", FromPort: 1000, ToPort: 1500, Protocol: "udp"},
	})
}

func (s *updateUnitPortsSuite) TestUpdateUnitPortsMatchingRangeAcrossEndpoints(c *tc.C) {
	// Arrange
	s.initialiseOpenPort(c)

	// Act
	s.runUpdateUnitPorts(c, s.unitUUID, network.GroupedPortRanges{"ep2": {{Protocol: "udp", FromPort: 1000, ToPort: 1500}}}, network.GroupedPortRanges{})

	// Assert
	obtainedPortRanges := s.getUnitOpenedPorts(c, s.unitUUID)
	c.Check(obtainedPortRanges, tc.SameContents, []endpointPortRange{
		{Endpoint: "ep0", FromPort: 80, ToPort: 80, Protocol: "tcp"},
		{Endpoint: "ep0", FromPort: 1000, ToPort: 1500, Protocol: "udp"},
		{Endpoint: "ep1", FromPort: 8080, ToPort: 8080, Protocol: "tcp"},
		{Endpoint: "ep2", FromPort: 1000, ToPort: 1500, Protocol: "udp"},
	})
}

func (s *updateUnitPortsSuite) TestUpdateUnitPortRangesCloseAlreadyClosed(c *tc.C) {
	// Arrange
	s.initialiseOpenPort(c)

	// Act
	s.runUpdateUnitPorts(c, s.unitUUID, network.GroupedPortRanges{}, network.GroupedPortRanges{
		"ep0": {{Protocol: "tcp", FromPort: 7000, ToPort: 7000}},
	})

	// Assert
	obtainedPortRanges := s.getUnitOpenedPorts(c, s.unitUUID)
	c.Check(obtainedPortRanges, tc.SameContents, []endpointPortRange{
		{Endpoint: "ep0", FromPort: 80, ToPort: 80, Protocol: "tcp"},
		{Endpoint: "ep0", FromPort: 1000, ToPort: 1500, Protocol: "udp"},
		{Endpoint: "ep1", FromPort: 8080, ToPort: 8080, Protocol: "tcp"},
	})
}

func (s *updateUnitPortsSuite) TestUpdateUnitPortRangeClosePortRangeWrongEndpoint(c *tc.C) {
	// Arrange
	s.initialiseOpenPort(c)

	// Act
	s.runUpdateUnitPorts(c, s.unitUUID, network.GroupedPortRanges{}, network.GroupedPortRanges{
		"ep1": {{Protocol: "tcp", FromPort: 80, ToPort: 80}},
	})

	// Assert
	obtainedPortRanges := s.getUnitOpenedPorts(c, s.unitUUID)
	c.Check(obtainedPortRanges, tc.SameContents, []endpointPortRange{
		{Endpoint: "ep0", FromPort: 80, ToPort: 80, Protocol: "tcp"},
		{Endpoint: "ep0", FromPort: 1000, ToPort: 1500, Protocol: "udp"},
		{Endpoint: "ep1", FromPort: 8080, ToPort: 8080, Protocol: "tcp"},
	})
}

func (s *updateUnitPortsSuite) TestUpdateUnitPortsOpenPortRangeAlreadyOpened(c *tc.C) {
	// Arrange
	s.initialiseOpenPort(c)

	// Act
	s.runUpdateUnitPorts(c, s.unitUUID, network.GroupedPortRanges{
		"ep0": {{Protocol: "tcp", FromPort: 80, ToPort: 80}},
	}, network.GroupedPortRanges{})

	// Assert
	obtainedPortRanges := s.getUnitOpenedPorts(c, s.unitUUID)
	c.Check(obtainedPortRanges, tc.SameContents, []endpointPortRange{
		{Endpoint: "ep0", FromPort: 80, ToPort: 80, Protocol: "tcp"},
		{Endpoint: "ep0", FromPort: 1000, ToPort: 1500, Protocol: "udp"},
		{Endpoint: "ep1", FromPort: 8080, ToPort: 8080, Protocol: "tcp"},
	})
}

func (s *updateUnitPortsSuite) TestUpdateUnitPortsSameRangeAcrossEndpoints(c *tc.C) {
	// Act
	s.runUpdateUnitPorts(c, s.unitUUID, network.GroupedPortRanges{
		"ep0": {network.MustParsePortRange("80/tcp"), network.MustParsePortRange("443/tcp")},
		"ep1": {network.MustParsePortRange("80/tcp")},
		"ep2": {network.MustParsePortRange("80/tcp")},
	}, network.GroupedPortRanges{})

	// Assert
	obtainedPortRanges := s.getUnitOpenedPorts(c, s.unitUUID)
	mc := tc.NewMultiChecker()
	mc.AddExpr("_.Protocol", tc.Equals, "tcp")
	c.Check(obtainedPortRanges, tc.UnorderedMatch[[]endpointPortRange](mc), []endpointPortRange{
		{Endpoint: "ep0", FromPort: 80, ToPort: 80},
		{Endpoint: "ep0", FromPort: 443, ToPort: 443},
		{Endpoint: "ep1", FromPort: 80, ToPort: 80},
		{Endpoint: "ep2", FromPort: 80, ToPort: 80},
	})
}

func (s *updateUnitPortsSuite) TestUpdateUnitPortsOpenPortConflictColocated(c *tc.C) {
	// Arrange: Open some co-located ports.
	unit1UUID, _ := s.createUnit(c, netNodeUUIDs[0], appNames[0])
	s.runUpdateUnitPorts(c, unit1UUID, network.GroupedPortRanges{
		"ep0": {
			network.MustParsePortRange("150-250/tcp"),
		},
	}, network.GroupedPortRanges{})

	// Act and Assert
	s.runUpdateUnitPortsWithError(c, s.unitUUID, network.GroupedPortRanges{
		"ep1": {
			network.MustParsePortRange("100-200/tcp"),
		},
	}, network.GroupedPortRanges{}, porterrors.PortRangeConflict)
}

func (s *updateUnitPortsSuite) TestUpdateUnitPortsClosePortConflictColocated(c *tc.C) {
	// Arrange: Open some co-located ports.
	unit1UUID, _ := s.createUnit(c, netNodeUUIDs[0], appNames[0])
	s.runUpdateUnitPorts(c, unit1UUID, network.GroupedPortRanges{
		"ep0": {
			network.MustParsePortRange("150-250/tcp"),
		},
	}, network.GroupedPortRanges{})

	// Act and Assert
	s.runUpdateUnitPortsWithError(c, s.unitUUID, network.GroupedPortRanges{}, network.GroupedPortRanges{
		"ep1": {
			network.MustParsePortRange("100-200/tcp"),
		},
	}, porterrors.PortRangeConflict)
}

func (s *updateUnitPortsSuite) TestUpdateUnitPortsOpenWildcard(c *tc.C) {
	// Arrange: Open port ranges on the specific endpoints.
	s.runUpdateUnitPorts(c, s.unitUUID, network.GroupedPortRanges{
		"ep0": {network.MustParsePortRange("100-200/tcp")},
		"ep1": {network.MustParsePortRange("100-200/tcp")},
		"ep2": {network.MustParsePortRange("100-200/tcp")},
	}, network.GroupedPortRanges{})

	// Act: Open port ranges on the wildcard endpoint and check the specific
	// endpoints are cleaned up.
	s.runUpdateUnitPorts(c, s.unitUUID, network.GroupedPortRanges{
		network.WildcardEndpoint: {network.MustParsePortRange("100-200/tcp")},
	}, network.GroupedPortRanges{})

	// Assert
	obtainedPortRanges := s.getUnitOpenedPorts(c, s.unitUUID)
	c.Check(obtainedPortRanges, tc.SameContents, []endpointPortRange{
		{Endpoint: network.WildcardEndpoint, FromPort: 100, ToPort: 200, Protocol: "tcp"},
	})
}

func (s *updateUnitPortsSuite) TestUpdateUnitPortsOpenPortRangeOpenOnWildcard(c *tc.C) {
	// Arrange: Open port ranges on the wildcard endpoint.
	s.runUpdateUnitPorts(c, s.unitUUID, network.GroupedPortRanges{
		network.WildcardEndpoint: {network.MustParsePortRange("100-200/tcp")},
	}, network.GroupedPortRanges{})

	// Act: Open port ranges on a specific endpoint and assert that
	// nothing happens.
	s.runUpdateUnitPorts(c, s.unitUUID, network.GroupedPortRanges{
		"ep0": {network.MustParsePortRange("100-200/tcp")},
	}, network.GroupedPortRanges{})

	// Assert
	obtainedPortRanges := s.getUnitOpenedPorts(c, s.unitUUID)
	c.Check(obtainedPortRanges, tc.SameContents, []endpointPortRange{
		{Endpoint: network.WildcardEndpoint, FromPort: 100, ToPort: 200, Protocol: "tcp"},
	})
}

func (s *updateUnitPortsSuite) TestUpdateUnitPortsCloseWildcard(c *tc.C) {
	// Arrange: Open some port ranges on specific endpoints.
	s.runUpdateUnitPorts(c, s.unitUUID, network.GroupedPortRanges{
		"ep0": {network.MustParsePortRange("100-200/tcp")},
		"ep1": {network.MustParsePortRange("100-200/tcp")},
		"ep2": {network.MustParsePortRange("100-200/tcp")},
	}, network.GroupedPortRanges{})

	// Act: Close the wildcard endpoint and check the specific endpoints are
	// cleaned up.
	s.runUpdateUnitPorts(c, s.unitUUID, network.GroupedPortRanges{}, network.GroupedPortRanges{
		network.WildcardEndpoint: {network.MustParsePortRange("100-200/tcp")},
	})

	// Assert
	obtainedPortRanges := s.getUnitOpenedPorts(c, s.unitUUID)
	c.Check(obtainedPortRanges, tc.HasLen, 0)
}

func (s *updateUnitPortsSuite) TestUpdateUnitPortsClosePortRangeOpenOnWildcard(c *tc.C) {
	// Arrange: Open port ranges on the wildcard endpoint.
	s.runUpdateUnitPorts(c, s.unitUUID, network.GroupedPortRanges{
		network.WildcardEndpoint: {network.MustParsePortRange("100-200/tcp")},
	}, network.GroupedPortRanges{})

	// Act: Close port ranges on a specific endpoint and assert that
	// nothing happens.
	s.runUpdateUnitPorts(c, s.unitUUID, network.GroupedPortRanges{}, network.GroupedPortRanges{
		"ep0": {network.MustParsePortRange("100-200/tcp")},
	})

	// Assert
	obtainedPortRanges := s.getUnitOpenedPorts(c, s.unitUUID)

	c.Check(obtainedPortRanges, tc.SameContents, []endpointPortRange{
		{Protocol: "tcp", FromPort: 100, ToPort: 200, Endpoint: relation.JujuInfo},
		{Protocol: "tcp", FromPort: 100, ToPort: 200, Endpoint: "ep1"},
		{Protocol: "tcp", FromPort: 100, ToPort: 200, Endpoint: "ep2"},
	})
}

func (s *updateUnitPortsSuite) TestUpdateUnitPortsOpenWildcardAndOtherRangeOnEndpoint(c *tc.C) {
	// Arrange: Open some port ranges on specific endpoints.
	s.runUpdateUnitPorts(c, s.unitUUID, network.GroupedPortRanges{
		"ep0": {network.MustParsePortRange("100-200/tcp")},
		"ep1": {network.MustParsePortRange("100-200/tcp")},
		"ep2": {network.MustParsePortRange("100-200/tcp")},
	}, network.GroupedPortRanges{})

	// Act: Open port ranges on the wildcard endpoint and check the specific
	// endpoints are cleaned up. Also, open another independent range on one
	// of the specific endpoints, and check that it is not affected.
	s.runUpdateUnitPorts(c, s.unitUUID, network.GroupedPortRanges{
		network.WildcardEndpoint: {network.MustParsePortRange("100-200/tcp")},
		"ep0":                    {network.MustParsePortRange("10-20/tcp")},
	}, network.GroupedPortRanges{})

	// Assert
	obtainedPortRanges := s.getUnitOpenedPorts(c, s.unitUUID)
	mc := tc.NewMultiChecker()
	mc.AddExpr("_.Protocol", tc.Equals, "tcp")
	c.Check(obtainedPortRanges, tc.UnorderedMatch[[]endpointPortRange](mc), []endpointPortRange{
		{Endpoint: "ep0", FromPort: 10, ToPort: 20},
		{Endpoint: network.WildcardEndpoint, FromPort: 100, ToPort: 200},
	})
}

func (s *updateUnitPortsSuite) TestUpdateUnitPortsOpenPortRangeOnWildcardAndOtherSameTime(c *tc.C) {
	// Act
	s.runUpdateUnitPorts(c, s.unitUUID, network.GroupedPortRanges{
		network.WildcardEndpoint: {network.MustParsePortRange("100-200/tcp")},
		"ep1":                    {network.MustParsePortRange("100-200/tcp")},
	}, network.GroupedPortRanges{})

	// Assert
	obtainedPortRanges := s.getUnitOpenedPorts(c, s.unitUUID)
	c.Check(obtainedPortRanges, tc.SameContents, []endpointPortRange{
		{Endpoint: network.WildcardEndpoint, FromPort: 100, ToPort: 200, Protocol: "tcp"},
	})
}

func (s *updateUnitPortsSuite) getUnitOpenedPorts(c *tc.C, unit string) []endpointPortRange {
	unitUUID := unitUUID{UUID: unit}

	query, err := s.state.Prepare(`
SELECT &endpointPortRange.*
FROM v_port_range
WHERE unit_uuid = $unitUUID.uuid
`, endpointPortRange{}, unitUUID)
	c.Assert(err, tc.IsNil)

	results := []endpointPortRange{}
	err = s.TxnRunner().Txn(c.Context(), func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, query, unitUUID).GetAll(&results)
		if errors.Is(err, sqlair.ErrNoRows) {
			return nil
		}
		return errors.Capture(err)
	})
	c.Assert(err, tc.IsNil)

	return results
}

func (s *updateUnitPortsSuite) getMachineOpenedPorts(c *tc.C, machine string) []unitEndpointPortRange {
	type machineUUID unitUUID
	mach := machineUUID{UUID: machine}

	query, err := s.state.Prepare(`
SELECT &unitEndpointPortRange.*
FROM v_port_range
JOIN unit ON unit_uuid = unit.uuid
JOIN machine ON unit.net_node_uuid = machine.net_node_uuid
WHERE machine.uuid = $machineUUID.uuid
`, unitEndpointPortRange{}, machineUUID{})
	c.Assert(err, tc.IsNil)

	results := []unitEndpointPortRange{}
	err = s.TxnRunner().Txn(c.Context(), func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, query, mach).GetAll(&results)
		if errors.Is(err, sqlair.ErrNoRows) {
			return nil
		}
		return errors.Capture(err)
	})
	c.Assert(err, tc.IsNil)
	return results
}

func (s *updateUnitPortsSuite) createApplicationWithRelations(c *tc.C, appName string, relations ...string) coreapplication.UUID {
	relationsMap := map[string]charm.Relation{}
	for _, relation := range relations {
		relationsMap[relation] = charm.Relation{
			Name:  relation,
			Role:  charm.RoleRequirer,
			Scope: charm.ScopeGlobal,
		}
	}

	applicationSt := applicationstate.NewState(s.TxnRunnerFactory(), model.UUID(s.ModelUUID()), clock.WallClock, loggertesting.WrapCheckLog(c))
	appUUID, _, err := applicationSt.CreateIAASApplication(c.Context(), appName, application.AddIAASApplicationArg{
		BaseAddApplicationArg: application.BaseAddApplicationArg{
			Charm: charm.Charm{
				Metadata: charm.Metadata{
					Name:     appName,
					Requires: relationsMap,
				},
				Manifest: charm.Manifest{
					Bases: []charm.Base{{
						Name:          "ubuntu",
						Channel:       charm.Channel{Risk: charm.RiskStable},
						Architectures: []string{"amd64"},
					}},
				},
				ReferenceName: appName,
				Architecture:  architecture.AMD64,
				Revision:      1,
				Source:        charm.LocalSource,
			},
			Constraints: constraints.Constraints{
				Arch: ptr(arch.AMD64),
			},
		},
	}, nil)
	c.Assert(err, tc.ErrorIsNil)
	return appUUID
}

// createUnit creates a new unit in state and returns its UUID. The unit is assigned
// to the net node with uuid `netNodeUUID` and application with name `appName`.
func (s *updateUnitPortsSuite) createUnit(c *tc.C, netNodeUUID, appName string) (string, string) {
	applicationSt := applicationstate.NewState(s.TxnRunnerFactory(), model.UUID(s.ModelUUID()), clock.WallClock, loggertesting.WrapCheckLog(c))

	appID, err := applicationSt.GetApplicationUUIDByName(c.Context(), appName)
	c.Assert(err, tc.ErrorIsNil)

	// Ensure that we place the unit on the same machine as the net node.
	var (
		machineUUID machine.UUID
		machineName machine.Name
	)
	err = s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		err := tx.QueryRowContext(ctx, `
SELECT uuid, name FROM machine WHERE net_node_uuid = ?
`, netNodeUUID).Scan(&machineUUID, &machineName)
		return err
	})
	c.Assert(err, tc.ErrorIsNil)

	unitNames, _, err := applicationSt.AddIAASUnits(c.Context(), appID, application.AddIAASUnitArg{
		MachineNetNodeUUID: domainnetwork.NetNodeUUID(netNodeUUID),
		MachineUUID:        machineUUID,
		AddUnitArg: application.AddUnitArg{
			NetNodeUUID: domainnetwork.NetNodeUUID(netNodeUUID),
			Placement: deployment.Placement{
				Type:      deployment.PlacementTypeMachine,
				Directive: machineName.String(),
			},
		},
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(unitNames, tc.HasLen, 1)
	unitName := unitNames[0].String()
	s.unitCount++

	var unitUUID string
	err = s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		err := tx.QueryRowContext(ctx, "SELECT uuid FROM unit WHERE name = ?", unitName).Scan(&unitUUID)
		if err != nil {
			return err
		}

		return nil
	})
	c.Assert(err, tc.ErrorIsNil)
	return unitUUID, unitName
}

// endpointPortRange represents a range of ports for a give protocol for a
// given endpoint.
type endpointPortRange struct {
	Protocol string `db:"protocol"`
	FromPort int    `db:"from_port"`
	ToPort   int    `db:"to_port"`
	Endpoint string `db:"endpoint"`
}

// unitEndpointPortRange represents a range of ports for a given protocol for
// a given unit's endpoint, and unit name.
type unitEndpointPortRange struct {
	UnitName string `db:"unit_name"`
	Protocol string `db:"protocol"`
	FromPort int    `db:"from_port"`
	ToPort   int    `db:"to_port"`
	Endpoint string `db:"endpoint"`
}
