// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"database/sql"
	"testing"

	"github.com/juju/clock"
	"github.com/juju/tc"

	coreapplication "github.com/juju/juju/core/application"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/core/network"
	coreunit "github.com/juju/juju/core/unit"
	"github.com/juju/juju/domain/deployment"
	domainmachine "github.com/juju/juju/domain/machine"
	machinestate "github.com/juju/juju/domain/machine/state"
	porterrors "github.com/juju/juju/domain/port/errors"
	"github.com/juju/juju/internal/logger"
	coretesting "github.com/juju/juju/internal/testing"
)

type importOpenUnitPortsSuite struct {
	baseSuite

	unitUUID coreunit.UUID
	unitName coreunit.Name

	appUUID coreapplication.UUID
}

func TestImportOpenUnitPortsSuite(t *testing.T) {
	tc.Run(t, &importOpenUnitPortsSuite{})
}

func (s *importOpenUnitPortsSuite) SetUpTest(c *tc.C) {
	s.ModelSuite.SetUpTest(c)

	modelUUID := tc.Must0(c, model.NewUUID)
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `
INSERT INTO model (uuid, controller_uuid, name, qualifier, type, cloud, cloud_type)
VALUES (?, ?, "test", "prod", "iaas", "test-model", "ec2")
		`, modelUUID.String(), coretesting.ControllerModelTag.Id())
		return err
	})
	c.Assert(err, tc.ErrorIsNil)

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
}

func (s *importOpenUnitPortsSuite) TestImportOpenUnitPortsOpenPort(c *tc.C) {
	st := NewState(s.TxnRunnerFactory())
	ctx := c.Context()

	err := st.ImportOpenUnitPorts(ctx, s.unitUUID, network.GroupedPortRanges{"ep0": {{Protocol: "tcp", FromPort: 1000, ToPort: 1500}}})
	c.Assert(err, tc.ErrorIsNil)

	groupedPortRanges, err := st.GetUnitOpenedPorts(ctx, s.unitUUID)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(groupedPortRanges, tc.HasLen, 1)
	c.Check(groupedPortRanges["ep0"], tc.HasLen, 1)
	c.Check(groupedPortRanges["ep0"][0], tc.DeepEquals, network.PortRange{Protocol: "tcp", FromPort: 1000, ToPort: 1500})
}

func (s *importOpenUnitPortsSuite) TestImportOpenUnitPortsOpenOnInvalidEndpoint(c *tc.C) {
	st := NewState(s.TxnRunnerFactory())
	ctx := c.Context()

	err := st.ImportOpenUnitPorts(ctx, s.unitUUID, network.GroupedPortRanges{
		"invalid": {{Protocol: "tcp", FromPort: 1000, ToPort: 1500}},
	})
	c.Assert(err, tc.ErrorIs, porterrors.InvalidEndpoint)
}

func (s *importOpenUnitPortsSuite) TestImportOpenUnitPortsOpenPortRangeMixedEndpoints(c *tc.C) {
	st := NewState(s.TxnRunnerFactory())
	ctx := c.Context()

	err := st.ImportOpenUnitPorts(ctx, s.unitUUID, network.GroupedPortRanges{
		"ep0": {{Protocol: "udp", FromPort: 2500, ToPort: 3000}},
		"ep2": {{Protocol: "udp", FromPort: 2000, ToPort: 2100}},
	})
	c.Assert(err, tc.ErrorIsNil)

	groupedPortRanges, err := st.GetUnitOpenedPorts(ctx, s.unitUUID)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(groupedPortRanges, tc.HasLen, 2)

	c.Check(groupedPortRanges["ep0"], tc.HasLen, 1)
	c.Check(groupedPortRanges["ep0"][0], tc.DeepEquals, network.PortRange{Protocol: "udp", FromPort: 2500, ToPort: 3000})

	c.Check(groupedPortRanges["ep2"], tc.HasLen, 1)
	c.Check(groupedPortRanges["ep2"][0], tc.DeepEquals, network.PortRange{Protocol: "udp", FromPort: 2000, ToPort: 2100})
}

func (s *importOpenUnitPortsSuite) TestUpdateUnitPortRangesOpenAlreadyOpenAcrossUnits(c *tc.C) {
	st := NewState(s.TxnRunnerFactory())
	ctx := c.Context()
	unit1UUID, unit1Name := s.createUnit(c, netNodeUUIDs[0], appNames[0])

	err := st.ImportOpenUnitPorts(ctx, s.unitUUID, network.GroupedPortRanges{"ep0": {{Protocol: "udp", FromPort: 1000, ToPort: 1500}}})
	c.Assert(err, tc.ErrorIsNil)

	err = st.ImportOpenUnitPorts(ctx, unit1UUID, network.GroupedPortRanges{"ep0": {{Protocol: "udp", FromPort: 1000, ToPort: 1500}}})
	c.Assert(err, tc.ErrorIsNil)

	machineGroupedPortRanges, err := st.GetMachineOpenedPorts(ctx, machineUUIDs[0])
	c.Assert(err, tc.ErrorIsNil)
	c.Check(machineGroupedPortRanges, tc.HasLen, 2)

	unit0PortRanges, ok := machineGroupedPortRanges[s.unitName]
	c.Assert(ok, tc.IsTrue)
	c.Check(unit0PortRanges["ep0"], tc.HasLen, 1)
	c.Check(unit0PortRanges["ep0"][0], tc.DeepEquals, network.PortRange{Protocol: "udp", FromPort: 1000, ToPort: 1500})

	unit1PortRanges, ok := machineGroupedPortRanges[unit1Name]
	c.Assert(ok, tc.IsTrue)
	c.Check(unit1PortRanges, tc.HasLen, 1)

	c.Check(unit1PortRanges["ep0"], tc.HasLen, 1)
	c.Check(unit1PortRanges["ep0"][0], tc.DeepEquals, network.PortRange{Protocol: "udp", FromPort: 1000, ToPort: 1500})
}

func (s *importOpenUnitPortsSuite) TestImportOpenUnitPortsNilOpenPort(c *tc.C) {
	st := NewState(s.TxnRunnerFactory())
	ctx := c.Context()

	err := st.ImportOpenUnitPorts(ctx, s.unitUUID, nil)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *importOpenUnitPortsSuite) TestImportOpenUnitPortsSameRangeAcrossEndpoints(c *tc.C) {
	st := NewState(s.TxnRunnerFactory())
	ctx := c.Context()

	err := st.ImportOpenUnitPorts(ctx, s.unitUUID, network.GroupedPortRanges{
		"ep0": {network.MustParsePortRange("80/tcp"), network.MustParsePortRange("443/tcp")},
		"ep1": {network.MustParsePortRange("80/tcp")},
		"ep2": {network.MustParsePortRange("80/tcp")},
	})
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
