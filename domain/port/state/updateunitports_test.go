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
	"github.com/juju/juju/core/machine"
	modeltesting "github.com/juju/juju/core/model/testing"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/relation"
	coreunit "github.com/juju/juju/core/unit"
	machinestate "github.com/juju/juju/domain/machine/state"
	porterrors "github.com/juju/juju/domain/port/errors"
	"github.com/juju/juju/internal/logger"
	coretesting "github.com/juju/juju/internal/testing"
)

type updateUnitPortsSuite struct {
	baseSuite

	unitUUID coreunit.UUID
	unitName coreunit.Name

	appUUID coreapplication.ID
}

func TestUpdateUnitPortsSuite(t *testing.T) {
	tc.Run(t, &updateUnitPortsSuite{})
}

func (s *updateUnitPortsSuite) SetUpTest(c *tc.C) {
	s.ModelSuite.SetUpTest(c)

	modelUUID := modeltesting.GenModelUUID(c)
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `
			INSERT INTO model (uuid, controller_uuid, name, qualifier, type, cloud, cloud_type)
			VALUES (?, ?, "test", "prod", "iaas", "test-model", "ec2")
		`, modelUUID.String(), coretesting.ControllerModelTag.Id())
		return err
	})
	c.Assert(err, tc.ErrorIsNil)

	machineSt := machinestate.NewState(s.TxnRunnerFactory(), clock.WallClock, logger.GetLogger("juju.test.machine"))
	err = machineSt.CreateMachine(c.Context(), "0", netNodeUUIDs[0], machine.UUID(machineUUIDs[0]))
	c.Assert(err, tc.ErrorIsNil)
	err = machineSt.CreateMachine(c.Context(), "1", netNodeUUIDs[1], machine.UUID(machineUUIDs[1]))
	c.Assert(err, tc.ErrorIsNil)

	s.appUUID = s.createApplicationWithRelations(c, appNames[0], "ep0", "ep1", "ep2")
	s.unitUUID, s.unitName = s.createUnit(c, netNodeUUIDs[0], appNames[0])
}

func (s *updateUnitPortsSuite) initialiseOpenPort(c *tc.C, st *State) {
	ctx := c.Context()
	err := st.UpdateUnitPorts(ctx, s.unitUUID, network.GroupedPortRanges{
		"ep0": {
			{Protocol: "tcp", FromPort: 80, ToPort: 80},
			{Protocol: "udp", FromPort: 1000, ToPort: 1500},
		},
		"ep1": {
			{Protocol: "tcp", FromPort: 8080, ToPort: 8080},
		},
	}, network.GroupedPortRanges{})
	c.Assert(err, tc.ErrorIsNil)
}

func (s *updateUnitPortsSuite) TestGetColocatedOpenedPortsSingleUnit(c *tc.C) {
	st := NewState(s.TxnRunnerFactory())
	ctx := c.Context()
	s.initialiseOpenPort(c, st)

	db, err := st.DB()
	c.Assert(err, tc.ErrorIsNil)

	var opendPorts []network.PortRange
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		var err error
		opendPorts, err = st.getColocatedOpenedPorts(ctx, tx, unitUUID{UUID: s.unitUUID})
		return err
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(opendPorts, tc.HasLen, 3)
	c.Check(opendPorts[0], tc.DeepEquals, network.PortRange{Protocol: "tcp", FromPort: 80, ToPort: 80})
	c.Check(opendPorts[1], tc.DeepEquals, network.PortRange{Protocol: "tcp", FromPort: 8080, ToPort: 8080})
	c.Check(opendPorts[2], tc.DeepEquals, network.PortRange{Protocol: "udp", FromPort: 1000, ToPort: 1500})
}

func (s *updateUnitPortsSuite) TestGetColocatedOpenedPortsMultipleUnits(c *tc.C) {
	st := NewState(s.TxnRunnerFactory())
	ctx := c.Context()
	s.initialiseOpenPort(c, st)

	unit1UUID, _ := s.createUnit(c, netNodeUUIDs[0], appNames[0])
	err := st.UpdateUnitPorts(ctx, unit1UUID, network.GroupedPortRanges{
		"ep0": {
			{Protocol: "tcp", FromPort: 443, ToPort: 443},
			{Protocol: "udp", FromPort: 2000, ToPort: 2500},
		},
	}, network.GroupedPortRanges{})
	c.Assert(err, tc.ErrorIsNil)

	db, err := st.DB()
	c.Assert(err, tc.ErrorIsNil)

	var opendPorts []network.PortRange
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		var err error
		opendPorts, err = st.getColocatedOpenedPorts(ctx, tx, unitUUID{UUID: s.unitUUID})
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
	st := NewState(s.TxnRunnerFactory())
	ctx := c.Context()
	s.initialiseOpenPort(c, st)

	unit1UUID, _ := s.createUnit(c, netNodeUUIDs[1], appNames[0])
	err := st.UpdateUnitPorts(ctx, unit1UUID, network.GroupedPortRanges{
		"ep0": {
			{Protocol: "tcp", FromPort: 443, ToPort: 443},
			{Protocol: "udp", FromPort: 2000, ToPort: 2500},
		},
	}, network.GroupedPortRanges{})
	c.Assert(err, tc.ErrorIsNil)

	db, err := st.DB()
	c.Assert(err, tc.ErrorIsNil)

	var opendPorts []network.PortRange
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		var err error
		opendPorts, err = st.getColocatedOpenedPorts(ctx, tx, unitUUID{UUID: s.unitUUID})
		return err
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(opendPorts, tc.HasLen, 3)
	c.Check(opendPorts[0], tc.DeepEquals, network.PortRange{Protocol: "tcp", FromPort: 80, ToPort: 80})
	c.Check(opendPorts[1], tc.DeepEquals, network.PortRange{Protocol: "tcp", FromPort: 8080, ToPort: 8080})
	c.Check(opendPorts[2], tc.DeepEquals, network.PortRange{Protocol: "udp", FromPort: 1000, ToPort: 1500})
}

func (s *updateUnitPortsSuite) TestGetWildcardEndpointOpenedPorts(c *tc.C) {
	st := NewState(s.TxnRunnerFactory())
	ctx := c.Context()

	err := st.UpdateUnitPorts(ctx, s.unitUUID, network.GroupedPortRanges{
		network.WildcardEndpoint: {network.MustParsePortRange("100-200/tcp")},
	}, network.GroupedPortRanges{})
	c.Assert(err, tc.ErrorIsNil)

	db, err := st.DB()
	c.Assert(err, tc.ErrorIsNil)

	var portRanges []network.PortRange
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		var err error
		portRanges, err = st.getWildcardEndpointOpenedPorts(ctx, tx, unitUUID{UUID: s.unitUUID})
		return err
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(portRanges, tc.HasLen, 1)
	c.Check(portRanges[0], tc.DeepEquals, network.MustParsePortRange("100-200/tcp"))
}

func (s *updateUnitPortsSuite) TestGetWildcardEndpointOpenedPortsIgnoresOtherEndpoints(c *tc.C) {
	st := NewState(s.TxnRunnerFactory())
	ctx := c.Context()
	s.initialiseOpenPort(c, st)

	db, err := st.DB()
	c.Assert(err, tc.ErrorIsNil)

	var portRanges []network.PortRange
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		var err error
		portRanges, err = st.getWildcardEndpointOpenedPorts(ctx, tx, unitUUID{UUID: s.unitUUID})
		return err
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(portRanges, tc.HasLen, 0)
}

func (s *updateUnitPortsSuite) TestGetEndpointsForPopulatedUnit(c *tc.C) {
	st := NewState(s.TxnRunnerFactory())
	ctx := c.Context()
	s.initialiseOpenPort(c, st)

	db, err := st.DB()
	c.Assert(err, tc.ErrorIsNil)

	var endpoints []string
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		var err error
		endpoints, err = st.getEndpoints(ctx, tx, unitUUID{UUID: s.unitUUID})
		return err
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(endpoints, tc.DeepEquals, []string{"ep0", "ep1", "ep2", relation.JujuInfo})
}

func (s *updateUnitPortsSuite) TestGetEndpointsForUnpopulatedUnit(c *tc.C) {
	st := NewState(s.TxnRunnerFactory())
	ctx := c.Context()

	db, err := st.DB()
	c.Assert(err, tc.ErrorIsNil)

	var endpoints []string
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		var err error
		endpoints, err = st.getEndpoints(ctx, tx, unitUUID{UUID: s.unitUUID})
		return err
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(endpoints, tc.DeepEquals, []string{"ep0", "ep1", "ep2", relation.JujuInfo})
}

func (s *updateUnitPortsSuite) TestUpdateUnitPortsOpenPort(c *tc.C) {
	st := NewState(s.TxnRunnerFactory())
	ctx := c.Context()
	s.initialiseOpenPort(c, st)

	err := st.UpdateUnitPorts(ctx, s.unitUUID, network.GroupedPortRanges{"ep0": {{Protocol: "tcp", FromPort: 1000, ToPort: 1500}}}, network.GroupedPortRanges{})
	c.Assert(err, tc.ErrorIsNil)

	groupedPortRanges, err := st.GetUnitOpenedPorts(ctx, s.unitUUID)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(groupedPortRanges, tc.HasLen, 2)
	c.Check(groupedPortRanges["ep0"], tc.HasLen, 3)
	c.Check(groupedPortRanges["ep0"][0], tc.DeepEquals, network.PortRange{Protocol: "tcp", FromPort: 80, ToPort: 80})
	c.Check(groupedPortRanges["ep0"][1], tc.DeepEquals, network.PortRange{Protocol: "tcp", FromPort: 1000, ToPort: 1500})
	c.Check(groupedPortRanges["ep0"][2], tc.DeepEquals, network.PortRange{Protocol: "udp", FromPort: 1000, ToPort: 1500})

	c.Check(groupedPortRanges["ep1"], tc.HasLen, 1)
	c.Check(groupedPortRanges["ep1"][0], tc.DeepEquals, network.PortRange{Protocol: "tcp", FromPort: 8080, ToPort: 8080})
}

func (s *updateUnitPortsSuite) TestUpdateUnitPortsOpenPortWildcardEndpoint(c *tc.C) {
	st := NewState(s.TxnRunnerFactory())
	ctx := c.Context()

	err := st.UpdateUnitPorts(ctx, s.unitUUID, network.GroupedPortRanges{
		network.WildcardEndpoint: {{Protocol: "tcp", FromPort: 1000, ToPort: 1500}},
	}, network.GroupedPortRanges{})
	c.Assert(err, tc.ErrorIsNil)

	groupedPortRanges, err := st.GetUnitOpenedPorts(ctx, s.unitUUID)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(groupedPortRanges, tc.HasLen, 1)
	c.Check(groupedPortRanges[network.WildcardEndpoint], tc.HasLen, 1)
	c.Check(groupedPortRanges[network.WildcardEndpoint][0], tc.DeepEquals, network.PortRange{Protocol: "tcp", FromPort: 1000, ToPort: 1500})
}

func (s *updateUnitPortsSuite) TestUpdateUnitPortsOpenOnInvalidEndpoint(c *tc.C) {
	st := NewState(s.TxnRunnerFactory())
	ctx := c.Context()

	err := st.UpdateUnitPorts(ctx, s.unitUUID, network.GroupedPortRanges{
		"invalid": {{Protocol: "tcp", FromPort: 1000, ToPort: 1500}},
	}, network.GroupedPortRanges{})
	c.Assert(err, tc.ErrorIs, porterrors.InvalidEndpoint)
}

func (s *updateUnitPortsSuite) TestUpdateUnitPortsClosePort(c *tc.C) {
	st := NewState(s.TxnRunnerFactory())
	ctx := c.Context()
	s.initialiseOpenPort(c, st)

	groupedPortRanges, err := st.GetUnitOpenedPorts(ctx, s.unitUUID)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(groupedPortRanges, tc.HasLen, 2)
	c.Check(groupedPortRanges["ep0"], tc.HasLen, 2)
	c.Check(groupedPortRanges["ep0"][0], tc.DeepEquals, network.PortRange{Protocol: "tcp", FromPort: 80, ToPort: 80})
	c.Check(groupedPortRanges["ep0"][1], tc.DeepEquals, network.PortRange{Protocol: "udp", FromPort: 1000, ToPort: 1500})

	c.Check(groupedPortRanges["ep1"], tc.HasLen, 1)
	c.Check(groupedPortRanges["ep1"][0], tc.DeepEquals, network.PortRange{Protocol: "tcp", FromPort: 8080, ToPort: 8080})

	err = st.UpdateUnitPorts(ctx, s.unitUUID, network.GroupedPortRanges{}, network.GroupedPortRanges{"ep0": {{Protocol: "tcp", FromPort: 80, ToPort: 80}}})
	c.Assert(err, tc.ErrorIsNil)

	groupedPortRanges, err = st.GetUnitOpenedPorts(ctx, s.unitUUID)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(groupedPortRanges, tc.HasLen, 2)
	c.Check(groupedPortRanges["ep0"], tc.HasLen, 1)
	c.Check(groupedPortRanges["ep0"][0], tc.DeepEquals, network.PortRange{Protocol: "udp", FromPort: 1000, ToPort: 1500})

	c.Check(groupedPortRanges["ep1"], tc.HasLen, 1)
	c.Check(groupedPortRanges["ep1"][0], tc.DeepEquals, network.PortRange{Protocol: "tcp", FromPort: 8080, ToPort: 8080})
}

func (s *updateUnitPortsSuite) TestUpdateUnitPortsOpenPortRangeAdjacent(c *tc.C) {
	st := NewState(s.TxnRunnerFactory())
	ctx := c.Context()
	s.initialiseOpenPort(c, st)

	err := st.UpdateUnitPorts(ctx, s.unitUUID, network.GroupedPortRanges{"ep0": {{Protocol: "udp", FromPort: 1501, ToPort: 2000}}}, network.GroupedPortRanges{})
	c.Assert(err, tc.ErrorIsNil)

	groupedPortRanges, err := st.GetUnitOpenedPorts(ctx, s.unitUUID)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(groupedPortRanges, tc.HasLen, 2)
	c.Check(groupedPortRanges["ep0"], tc.HasLen, 3)
	c.Check(groupedPortRanges["ep0"][0], tc.DeepEquals, network.PortRange{Protocol: "tcp", FromPort: 80, ToPort: 80})
	c.Check(groupedPortRanges["ep0"][1], tc.DeepEquals, network.PortRange{Protocol: "udp", FromPort: 1000, ToPort: 1500})
	c.Check(groupedPortRanges["ep0"][2], tc.DeepEquals, network.PortRange{Protocol: "udp", FromPort: 1501, ToPort: 2000})

	c.Check(groupedPortRanges["ep1"], tc.HasLen, 1)
	c.Check(groupedPortRanges["ep1"][0], tc.DeepEquals, network.PortRange{Protocol: "tcp", FromPort: 8080, ToPort: 8080})
}

func (s *updateUnitPortsSuite) TestUpdateUnitPortsClosePortRange(c *tc.C) {
	st := NewState(s.TxnRunnerFactory())
	ctx := c.Context()
	s.initialiseOpenPort(c, st)

	err := st.UpdateUnitPorts(ctx, s.unitUUID, network.GroupedPortRanges{}, network.GroupedPortRanges{"ep0": {{Protocol: "udp", FromPort: 1000, ToPort: 1500}}})
	c.Assert(err, tc.ErrorIsNil)

	groupedPortRanges, err := st.GetUnitOpenedPorts(ctx, s.unitUUID)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(groupedPortRanges, tc.HasLen, 2)
	c.Check(groupedPortRanges["ep0"], tc.HasLen, 1)
	c.Check(groupedPortRanges["ep0"][0], tc.DeepEquals, network.PortRange{Protocol: "tcp", FromPort: 80, ToPort: 80})

	c.Check(groupedPortRanges["ep1"], tc.HasLen, 1)
	c.Check(groupedPortRanges["ep1"][0], tc.DeepEquals, network.PortRange{Protocol: "tcp", FromPort: 8080, ToPort: 8080})
}

func (s *updateUnitPortsSuite) TestUpdateUnitPortsClosePortEndpoint(c *tc.C) {
	st := NewState(s.TxnRunnerFactory())
	ctx := c.Context()
	s.initialiseOpenPort(c, st)

	err := st.UpdateUnitPorts(ctx, s.unitUUID, network.GroupedPortRanges{}, network.GroupedPortRanges{
		"ep0": {
			{Protocol: "tcp", FromPort: 80, ToPort: 80},
			{Protocol: "udp", FromPort: 1000, ToPort: 1500},
		},
	})
	c.Assert(err, tc.ErrorIsNil)

	groupedPortRanges, err := st.GetUnitOpenedPorts(ctx, s.unitUUID)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(groupedPortRanges, tc.HasLen, 1)

	c.Check(groupedPortRanges["ep1"], tc.HasLen, 1)
	c.Check(groupedPortRanges["ep1"][0], tc.DeepEquals, network.PortRange{Protocol: "tcp", FromPort: 8080, ToPort: 8080})
}

func (s *updateUnitPortsSuite) TestUpdateUnitPortsOpenCloseICMP(c *tc.C) {
	st := NewState(s.TxnRunnerFactory())
	ctx := c.Context()
	s.initialiseOpenPort(c, st)

	err := st.UpdateUnitPorts(ctx, s.unitUUID, network.GroupedPortRanges{"ep0": {{Protocol: "icmp"}}}, network.GroupedPortRanges{})
	c.Assert(err, tc.ErrorIsNil)

	groupedPortRanges, err := st.GetUnitOpenedPorts(ctx, s.unitUUID)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(groupedPortRanges, tc.HasLen, 2)
	c.Check(groupedPortRanges["ep0"], tc.HasLen, 3)
	c.Check(groupedPortRanges["ep0"][0], tc.DeepEquals, network.PortRange{Protocol: "icmp"})
	c.Check(groupedPortRanges["ep0"][1], tc.DeepEquals, network.PortRange{Protocol: "tcp", FromPort: 80, ToPort: 80})
	c.Check(groupedPortRanges["ep0"][2], tc.DeepEquals, network.PortRange{Protocol: "udp", FromPort: 1000, ToPort: 1500})

	c.Check(groupedPortRanges["ep1"], tc.HasLen, 1)
	c.Check(groupedPortRanges["ep1"][0], tc.DeepEquals, network.PortRange{Protocol: "tcp", FromPort: 8080, ToPort: 8080})

	err = st.UpdateUnitPorts(ctx, s.unitUUID, network.GroupedPortRanges{}, network.GroupedPortRanges{"ep0": {{Protocol: "icmp"}}})
	c.Check(err, tc.ErrorIsNil)

	groupedPortRanges, err = st.GetUnitOpenedPorts(ctx, s.unitUUID)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(groupedPortRanges, tc.HasLen, 2)
	c.Check(groupedPortRanges["ep0"], tc.HasLen, 2)
	c.Check(groupedPortRanges["ep0"][0], tc.DeepEquals, network.PortRange{Protocol: "tcp", FromPort: 80, ToPort: 80})
	c.Check(groupedPortRanges["ep0"][1], tc.DeepEquals, network.PortRange{Protocol: "udp", FromPort: 1000, ToPort: 1500})

	c.Check(groupedPortRanges["ep1"], tc.HasLen, 1)
	c.Check(groupedPortRanges["ep1"][0], tc.DeepEquals, network.PortRange{Protocol: "tcp", FromPort: 8080, ToPort: 8080})
}

func (s *updateUnitPortsSuite) TestUpdateUnitPortsOpenPortRangeMixedEndpoints(c *tc.C) {
	st := NewState(s.TxnRunnerFactory())
	ctx := c.Context()
	s.initialiseOpenPort(c, st)

	err := st.UpdateUnitPorts(ctx, s.unitUUID, network.GroupedPortRanges{
		"ep0": {{Protocol: "udp", FromPort: 2500, ToPort: 3000}},
		"ep2": {{Protocol: "udp", FromPort: 2000, ToPort: 2100}},
	}, network.GroupedPortRanges{})
	c.Assert(err, tc.ErrorIsNil)

	groupedPortRanges, err := st.GetUnitOpenedPorts(ctx, s.unitUUID)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(groupedPortRanges, tc.HasLen, 3)

	c.Check(groupedPortRanges["ep0"], tc.HasLen, 3)
	c.Check(groupedPortRanges["ep0"][0], tc.DeepEquals, network.PortRange{Protocol: "tcp", FromPort: 80, ToPort: 80})
	c.Check(groupedPortRanges["ep0"][1], tc.DeepEquals, network.PortRange{Protocol: "udp", FromPort: 1000, ToPort: 1500})
	c.Check(groupedPortRanges["ep0"][2], tc.DeepEquals, network.PortRange{Protocol: "udp", FromPort: 2500, ToPort: 3000})

	c.Check(groupedPortRanges["ep2"], tc.HasLen, 1)
	c.Check(groupedPortRanges["ep2"][0], tc.DeepEquals, network.PortRange{Protocol: "udp", FromPort: 2000, ToPort: 2100})

	c.Check(groupedPortRanges["ep1"], tc.HasLen, 1)
	c.Check(groupedPortRanges["ep1"][0], tc.DeepEquals, network.PortRange{Protocol: "tcp", FromPort: 8080, ToPort: 8080})
}

func (s *updateUnitPortsSuite) TestUpdateUnitPortsClosePortRangeMixedEndpoints(c *tc.C) {
	st := NewState(s.TxnRunnerFactory())
	ctx := c.Context()
	s.initialiseOpenPort(c, st)

	err := st.UpdateUnitPorts(ctx, s.unitUUID, network.GroupedPortRanges{
		"ep2": {
			{Protocol: "udp", FromPort: 2000, ToPort: 2500},
			{Protocol: "udp", FromPort: 3000, ToPort: 3000},
		},
	}, network.GroupedPortRanges{})
	c.Assert(err, tc.ErrorIsNil)

	err = st.UpdateUnitPorts(ctx, s.unitUUID, network.GroupedPortRanges{}, network.GroupedPortRanges{
		"ep0": {{Protocol: "udp", FromPort: 1000, ToPort: 1500}},
		"ep2": {{Protocol: "udp", FromPort: 2000, ToPort: 2500}},
	})
	c.Assert(err, tc.ErrorIsNil)

	groupedPortRanges, err := st.GetUnitOpenedPorts(ctx, s.unitUUID)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(groupedPortRanges, tc.HasLen, 3)

	c.Check(groupedPortRanges["ep0"], tc.HasLen, 1)
	c.Check(groupedPortRanges["ep0"][0], tc.DeepEquals, network.PortRange{Protocol: "tcp", FromPort: 80, ToPort: 80})

	c.Check(groupedPortRanges["ep2"], tc.HasLen, 1)
	c.Check(groupedPortRanges["ep2"][0], tc.DeepEquals, network.PortRange{Protocol: "udp", FromPort: 3000, ToPort: 3000})

	c.Check(groupedPortRanges["ep1"], tc.HasLen, 1)
	c.Check(groupedPortRanges["ep1"][0], tc.DeepEquals, network.PortRange{Protocol: "tcp", FromPort: 8080, ToPort: 8080})
}

func (s *updateUnitPortsSuite) TestUpdateUnitPortRangesOpenAlreadyOpenAcrossUnits(c *tc.C) {
	st := NewState(s.TxnRunnerFactory())
	ctx := c.Context()
	s.initialiseOpenPort(c, st)
	unit1UUID, unit1Name := s.createUnit(c, netNodeUUIDs[0], appNames[0])

	err := st.UpdateUnitPorts(ctx, s.unitUUID, network.GroupedPortRanges{"ep0": {{Protocol: "udp", FromPort: 1000, ToPort: 1500}}}, network.GroupedPortRanges{})
	c.Assert(err, tc.ErrorIsNil)

	err = st.UpdateUnitPorts(ctx, unit1UUID, network.GroupedPortRanges{"ep0": {{Protocol: "udp", FromPort: 1000, ToPort: 1500}}}, network.GroupedPortRanges{})
	c.Assert(err, tc.ErrorIsNil)

	machineGroupedPortRanges, err := st.GetMachineOpenedPorts(ctx, machineUUIDs[0])
	c.Assert(err, tc.ErrorIsNil)
	c.Check(machineGroupedPortRanges, tc.HasLen, 2)

	unit0PortRanges, ok := machineGroupedPortRanges[s.unitName]
	c.Assert(ok, tc.IsTrue)
	c.Check(unit0PortRanges["ep0"], tc.HasLen, 2)
	c.Check(unit0PortRanges["ep0"][0], tc.DeepEquals, network.PortRange{Protocol: "tcp", FromPort: 80, ToPort: 80})
	c.Check(unit0PortRanges["ep0"][1], tc.DeepEquals, network.PortRange{Protocol: "udp", FromPort: 1000, ToPort: 1500})

	unit1PortRanges, ok := machineGroupedPortRanges[unit1Name]
	c.Assert(ok, tc.IsTrue)
	c.Check(unit1PortRanges, tc.HasLen, 1)

	c.Check(unit1PortRanges["ep0"], tc.HasLen, 1)
	c.Check(unit1PortRanges["ep0"][0], tc.DeepEquals, network.PortRange{Protocol: "udp", FromPort: 1000, ToPort: 1500})
}

func (s *updateUnitPortsSuite) TestUpdateUnitPortsMatchingRangeAcrossEndpoints(c *tc.C) {
	st := NewState(s.TxnRunnerFactory())
	ctx := c.Context()
	s.initialiseOpenPort(c, st)

	err := st.UpdateUnitPorts(ctx, s.unitUUID, network.GroupedPortRanges{"ep2": {{Protocol: "udp", FromPort: 1000, ToPort: 1500}}}, network.GroupedPortRanges{})
	c.Assert(err, tc.ErrorIsNil)

	groupedPortRanges, err := st.GetUnitOpenedPorts(ctx, s.unitUUID)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(groupedPortRanges, tc.HasLen, 3)

	c.Check(groupedPortRanges["ep0"], tc.HasLen, 2)
	c.Check(groupedPortRanges["ep0"][0], tc.DeepEquals, network.PortRange{Protocol: "tcp", FromPort: 80, ToPort: 80})
	c.Check(groupedPortRanges["ep0"][1], tc.DeepEquals, network.PortRange{Protocol: "udp", FromPort: 1000, ToPort: 1500})

	c.Check(groupedPortRanges["ep2"], tc.HasLen, 1)
	c.Check(groupedPortRanges["ep2"][0], tc.DeepEquals, network.PortRange{Protocol: "udp", FromPort: 1000, ToPort: 1500})

	c.Check(groupedPortRanges["ep1"], tc.HasLen, 1)
	c.Check(groupedPortRanges["ep1"][0], tc.DeepEquals, network.PortRange{Protocol: "tcp", FromPort: 8080, ToPort: 8080})
}

func (s *updateUnitPortsSuite) TestUpdateUnitPortRangesCloseAlreadyClosed(c *tc.C) {
	st := NewState(s.TxnRunnerFactory())
	ctx := c.Context()
	s.initialiseOpenPort(c, st)

	err := st.UpdateUnitPorts(ctx, s.unitUUID, network.GroupedPortRanges{}, network.GroupedPortRanges{
		"ep0": {{Protocol: "tcp", FromPort: 7000, ToPort: 7000}},
	})
	c.Assert(err, tc.ErrorIsNil)

	groupedPortRanges, err := st.GetUnitOpenedPorts(ctx, s.unitUUID)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(groupedPortRanges, tc.HasLen, 2)

	c.Check(groupedPortRanges["ep0"], tc.HasLen, 2)
	c.Check(groupedPortRanges["ep0"][0], tc.DeepEquals, network.PortRange{Protocol: "tcp", FromPort: 80, ToPort: 80})
	c.Check(groupedPortRanges["ep0"][1], tc.DeepEquals, network.PortRange{Protocol: "udp", FromPort: 1000, ToPort: 1500})

	c.Check(groupedPortRanges["ep1"], tc.HasLen, 1)
	c.Check(groupedPortRanges["ep1"][0], tc.DeepEquals, network.PortRange{Protocol: "tcp", FromPort: 8080, ToPort: 8080})
}

func (s *updateUnitPortsSuite) TestUpdateUnitPortRangeClosePortRangeWrongEndpoint(c *tc.C) {
	st := NewState(s.TxnRunnerFactory())
	ctx := c.Context()
	s.initialiseOpenPort(c, st)

	err := st.UpdateUnitPorts(ctx, s.unitUUID, network.GroupedPortRanges{}, network.GroupedPortRanges{
		"ep1": {{Protocol: "tcp", FromPort: 80, ToPort: 80}},
	})
	c.Check(err, tc.ErrorIsNil)

	groupedPortRanges, err := st.GetUnitOpenedPorts(ctx, s.unitUUID)
	c.Check(err, tc.ErrorIsNil)
	c.Check(groupedPortRanges, tc.HasLen, 2)

	c.Check(groupedPortRanges["ep0"], tc.HasLen, 2)
	c.Check(groupedPortRanges["ep0"][0], tc.DeepEquals, network.PortRange{Protocol: "tcp", FromPort: 80, ToPort: 80})
	c.Check(groupedPortRanges["ep0"][1], tc.DeepEquals, network.PortRange{Protocol: "udp", FromPort: 1000, ToPort: 1500})

	c.Check(groupedPortRanges["ep1"], tc.HasLen, 1)
	c.Check(groupedPortRanges["ep1"][0], tc.DeepEquals, network.PortRange{Protocol: "tcp", FromPort: 8080, ToPort: 8080})
}

func (s *updateUnitPortsSuite) TestUpdateUnitPortsOpenPortRangeAlreadyOpened(c *tc.C) {
	st := NewState(s.TxnRunnerFactory())
	ctx := c.Context()
	s.initialiseOpenPort(c, st)

	err := st.UpdateUnitPorts(ctx, s.unitUUID, network.GroupedPortRanges{"ep0": {{Protocol: "tcp", FromPort: 80, ToPort: 80}}}, network.GroupedPortRanges{})
	c.Assert(err, tc.ErrorIsNil)

	groupedPortRanges, err := st.GetUnitOpenedPorts(ctx, s.unitUUID)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(groupedPortRanges, tc.HasLen, 2)

	c.Check(groupedPortRanges["ep0"], tc.HasLen, 2)
	c.Check(groupedPortRanges["ep0"][0], tc.DeepEquals, network.PortRange{Protocol: "tcp", FromPort: 80, ToPort: 80})
	c.Check(groupedPortRanges["ep0"][1], tc.DeepEquals, network.PortRange{Protocol: "udp", FromPort: 1000, ToPort: 1500})

	c.Check(groupedPortRanges["ep1"], tc.HasLen, 1)
	c.Check(groupedPortRanges["ep1"][0], tc.DeepEquals, network.PortRange{Protocol: "tcp", FromPort: 8080, ToPort: 8080})
}

func (s *updateUnitPortsSuite) TestUpdateUnitPortsNilOpenPort(c *tc.C) {
	st := NewState(s.TxnRunnerFactory())
	ctx := c.Context()

	err := st.UpdateUnitPorts(ctx, s.unitUUID, nil, nil)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *updateUnitPortsSuite) TestUpdateUnitPortsSameRangeAcrossEndpoints(c *tc.C) {
	st := NewState(s.TxnRunnerFactory())
	ctx := c.Context()

	err := st.UpdateUnitPorts(ctx, s.unitUUID, network.GroupedPortRanges{
		"ep0": {network.MustParsePortRange("80/tcp"), network.MustParsePortRange("443/tcp")},
		"ep1": {network.MustParsePortRange("80/tcp")},
		"ep2": {network.MustParsePortRange("80/tcp")},
	}, network.GroupedPortRanges{})
	c.Assert(err, tc.ErrorIsNil)

	groupedPortRanges, err := st.GetUnitOpenedPorts(ctx, s.unitUUID)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(groupedPortRanges, tc.HasLen, 3)

	c.Check(groupedPortRanges["ep0"], tc.HasLen, 2)
	c.Check(groupedPortRanges["ep0"][0], tc.DeepEquals, network.PortRange{Protocol: "tcp", FromPort: 80, ToPort: 80})
	c.Check(groupedPortRanges["ep0"][1], tc.DeepEquals, network.PortRange{Protocol: "tcp", FromPort: 443, ToPort: 443})

	c.Check(groupedPortRanges["ep1"], tc.HasLen, 1)
	c.Check(groupedPortRanges["ep1"][0], tc.DeepEquals, network.PortRange{Protocol: "tcp", FromPort: 80, ToPort: 80})

	c.Check(groupedPortRanges["ep2"], tc.HasLen, 1)
	c.Check(groupedPortRanges["ep2"][0], tc.DeepEquals, network.PortRange{Protocol: "tcp", FromPort: 80, ToPort: 80})
}

func (s *updateUnitPortsSuite) TestUpdateUnitPortsOpenPortConflictColocated(c *tc.C) {
	st := NewState(s.TxnRunnerFactory())
	ctx := c.Context()

	// Open some co-located ports.
	unit1UUID, _ := s.createUnit(c, netNodeUUIDs[0], appNames[0])
	err := st.UpdateUnitPorts(ctx, unit1UUID, network.GroupedPortRanges{
		"ep0": {
			network.MustParsePortRange("150-250/tcp"),
		},
	}, network.GroupedPortRanges{})
	c.Assert(err, tc.ErrorIsNil)

	err = st.UpdateUnitPorts(ctx, s.unitUUID, network.GroupedPortRanges{
		"ep1": {
			network.MustParsePortRange("100-200/tcp"),
		},
	}, network.GroupedPortRanges{})

	c.Assert(err, tc.ErrorIs, porterrors.PortRangeConflict)
}

func (s *updateUnitPortsSuite) TestUpdateUnitPortsClosePortConflictColocated(c *tc.C) {
	st := NewState(s.TxnRunnerFactory())
	ctx := c.Context()

	// Open some co-located ports.
	unit1UUID, _ := s.createUnit(c, netNodeUUIDs[0], appNames[0])
	err := st.UpdateUnitPorts(ctx, unit1UUID, network.GroupedPortRanges{
		"ep0": {
			network.MustParsePortRange("150-250/tcp"),
		},
	}, network.GroupedPortRanges{})
	c.Assert(err, tc.ErrorIsNil)

	err = st.UpdateUnitPorts(ctx, s.unitUUID, network.GroupedPortRanges{}, network.GroupedPortRanges{
		"ep1": {
			network.MustParsePortRange("100-200/tcp"),
		},
	})

	c.Assert(err, tc.ErrorIs, porterrors.PortRangeConflict)
}

func (s *updateUnitPortsSuite) TestUpdateUnitPortsOpenWildcard(c *tc.C) {
	st := NewState(s.TxnRunnerFactory())
	ctx := c.Context()

	// Open port ranges on the specific endpoints.
	err := st.UpdateUnitPorts(ctx, s.unitUUID, network.GroupedPortRanges{
		"ep0": {network.MustParsePortRange("100-200/tcp")},
		"ep1": {network.MustParsePortRange("100-200/tcp")},
		"ep2": {network.MustParsePortRange("100-200/tcp")},
	}, network.GroupedPortRanges{})
	c.Assert(err, tc.ErrorIsNil)

	// Open port ranges on the wildcard endpoint and check the specific endpoints
	// are cleaned up
	err = st.UpdateUnitPorts(ctx, s.unitUUID, network.GroupedPortRanges{
		network.WildcardEndpoint: {network.MustParsePortRange("100-200/tcp")},
	}, network.GroupedPortRanges{})
	c.Assert(err, tc.ErrorIsNil)

	groupedPortRanges, err := st.GetUnitOpenedPorts(ctx, s.unitUUID)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(groupedPortRanges, tc.HasLen, 1)

	c.Check(groupedPortRanges[network.WildcardEndpoint], tc.HasLen, 1)
	c.Check(groupedPortRanges[network.WildcardEndpoint][0], tc.DeepEquals, network.MustParsePortRange("100-200/tcp"))
}

func (s *updateUnitPortsSuite) TestUpdateUnitPortsOpenPortRangeOpenOnWildcard(c *tc.C) {
	st := NewState(s.TxnRunnerFactory())
	ctx := c.Context()

	// Open port ranges on the wildcard endpoint.
	err := st.UpdateUnitPorts(ctx, s.unitUUID, network.GroupedPortRanges{
		network.WildcardEndpoint: {network.MustParsePortRange("100-200/tcp")},
	}, network.GroupedPortRanges{})
	c.Assert(err, tc.ErrorIsNil)

	// Open port ranges on a specific endpoint and assert that nothing happens
	err = st.UpdateUnitPorts(ctx, s.unitUUID, network.GroupedPortRanges{
		"ep0": {network.MustParsePortRange("100-200/tcp")},
	}, network.GroupedPortRanges{})
	c.Assert(err, tc.ErrorIsNil)

	groupedPortRanges, err := st.GetUnitOpenedPorts(ctx, s.unitUUID)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(groupedPortRanges, tc.HasLen, 1)

	c.Check(groupedPortRanges[network.WildcardEndpoint], tc.HasLen, 1)
	c.Check(groupedPortRanges[network.WildcardEndpoint][0], tc.DeepEquals, network.MustParsePortRange("100-200/tcp"))
}

func (s *updateUnitPortsSuite) TestUpdateUnitPortsCloseWildcard(c *tc.C) {
	st := NewState(s.TxnRunnerFactory())
	ctx := c.Context()

	// Open some port ranges on specific endpoints.
	err := st.UpdateUnitPorts(ctx, s.unitUUID, network.GroupedPortRanges{
		"ep0": {network.MustParsePortRange("100-200/tcp")},
		"ep1": {network.MustParsePortRange("100-200/tcp")},
		"ep2": {network.MustParsePortRange("100-200/tcp")},
	}, network.GroupedPortRanges{})
	c.Assert(err, tc.ErrorIsNil)

	// Close the wildcard endpoint and check the specific endpoints are cleaned up.
	err = st.UpdateUnitPorts(ctx, s.unitUUID, network.GroupedPortRanges{}, network.GroupedPortRanges{
		network.WildcardEndpoint: {network.MustParsePortRange("100-200/tcp")},
	})
	c.Assert(err, tc.ErrorIsNil)

	groupedPortRanges, err := st.GetUnitOpenedPorts(ctx, s.unitUUID)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(groupedPortRanges, tc.HasLen, 0)
}

func (s *updateUnitPortsSuite) TestUpdateUnitPortsClosePortRangeOpenOnWildcard(c *tc.C) {
	st := NewState(s.TxnRunnerFactory())
	ctx := c.Context()

	// Open port ranges on the wildcard endpoint.
	err := st.UpdateUnitPorts(ctx, s.unitUUID, network.GroupedPortRanges{
		network.WildcardEndpoint: {network.MustParsePortRange("100-200/tcp")},
	}, network.GroupedPortRanges{})
	c.Assert(err, tc.ErrorIsNil)

	// Close port ranges on a specific endpoint and assert that nothing happens
	err = st.UpdateUnitPorts(ctx, s.unitUUID, network.GroupedPortRanges{}, network.GroupedPortRanges{
		"ep0": {network.MustParsePortRange("100-200/tcp")},
	})
	c.Assert(err, tc.ErrorIsNil)

	groupedPortRanges, err := st.GetUnitOpenedPorts(ctx, s.unitUUID)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(groupedPortRanges, tc.HasLen, 3)

	c.Check(groupedPortRanges["ep1"], tc.HasLen, 1)
	c.Check(groupedPortRanges["ep1"][0], tc.DeepEquals, network.MustParsePortRange("100-200/tcp"))

	c.Check(groupedPortRanges["ep2"], tc.HasLen, 1)
	c.Check(groupedPortRanges["ep2"][0], tc.DeepEquals, network.MustParsePortRange("100-200/tcp"))

	c.Check(groupedPortRanges[relation.JujuInfo], tc.HasLen, 1)
	c.Check(groupedPortRanges[relation.JujuInfo][0], tc.DeepEquals, network.MustParsePortRange("100-200/tcp"))
}

func (s *updateUnitPortsSuite) TestUpdateUnitPortsOpenWildcardAndOtherRangeOnEndpoint(c *tc.C) {
	st := NewState(s.TxnRunnerFactory())
	ctx := c.Context()

	// Open some port ranges on specific endpoints.
	err := st.UpdateUnitPorts(ctx, s.unitUUID, network.GroupedPortRanges{
		"ep0": {network.MustParsePortRange("100-200/tcp")},
		"ep1": {network.MustParsePortRange("100-200/tcp")},
		"ep2": {network.MustParsePortRange("100-200/tcp")},
	}, network.GroupedPortRanges{})
	c.Assert(err, tc.ErrorIsNil)

	// Open port ranges on the wildcard endpoint and check the specific endpoints
	// are cleaned up. Also, open another independent range on one of the specific
	// endpoints, and check that it is not affected.
	err = st.UpdateUnitPorts(ctx, s.unitUUID, network.GroupedPortRanges{
		network.WildcardEndpoint: {network.MustParsePortRange("100-200/tcp")},
		"ep0":                    {network.MustParsePortRange("10-20/tcp")},
	}, network.GroupedPortRanges{})
	c.Assert(err, tc.ErrorIsNil)

	groupedPortRanges, err := st.GetUnitOpenedPorts(ctx, s.unitUUID)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(groupedPortRanges, tc.HasLen, 2)

	c.Check(groupedPortRanges[network.WildcardEndpoint], tc.HasLen, 1)
	c.Check(groupedPortRanges[network.WildcardEndpoint][0], tc.DeepEquals, network.MustParsePortRange("100-200/tcp"))

	c.Check(groupedPortRanges["ep0"], tc.HasLen, 1)
	c.Check(groupedPortRanges["ep0"][0], tc.DeepEquals, network.MustParsePortRange("10-20/tcp"))
}

func (s *updateUnitPortsSuite) TestUpdateUnitPortsOpenPortRangeOnWildcardAndOtherSameTime(c *tc.C) {
	st := NewState(s.TxnRunnerFactory())
	ctx := c.Context()

	err := st.UpdateUnitPorts(ctx, s.unitUUID, network.GroupedPortRanges{
		network.WildcardEndpoint: {network.MustParsePortRange("100-200/tcp")},
		"ep1":                    {network.MustParsePortRange("100-200/tcp")},
	},
		network.GroupedPortRanges{},
	)
	c.Assert(err, tc.ErrorIsNil)

	groupedPortRanges, err := st.GetUnitOpenedPorts(ctx, s.unitUUID)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(groupedPortRanges, tc.HasLen, 1)

	c.Check(groupedPortRanges[network.WildcardEndpoint], tc.HasLen, 1)
	c.Check(groupedPortRanges[network.WildcardEndpoint][0], tc.DeepEquals, network.MustParsePortRange("100-200/tcp"))
}
