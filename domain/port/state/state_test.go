// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"database/sql"
	stdtesting "testing"

	"github.com/juju/clock"
	"github.com/juju/tc"

	coreapplication "github.com/juju/juju/core/application"
	"github.com/juju/juju/core/machine"
	modeltesting "github.com/juju/juju/core/model/testing"
	"github.com/juju/juju/core/network"
	coreunit "github.com/juju/juju/core/unit"
	"github.com/juju/juju/domain/application"
	"github.com/juju/juju/domain/application/architecture"
	"github.com/juju/juju/domain/application/charm"
	applicationstate "github.com/juju/juju/domain/application/state"
	"github.com/juju/juju/domain/deployment"
	machinestate "github.com/juju/juju/domain/machine/state"
	"github.com/juju/juju/domain/port"
	porterrors "github.com/juju/juju/domain/port/errors"
	"github.com/juju/juju/internal/changestream/testing"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	coretesting "github.com/juju/juju/internal/testing"
)

type baseSuite struct {
	testing.ModelSuite
	unitCount int
}

type stateSuite struct {
	baseSuite

	unitUUID coreunit.UUID
	unitName coreunit.Name

	appUUID coreapplication.ID
}

func TestStateSuite(t *stdtesting.T) { tc.Run(t, &stateSuite{}) }

var (
	machineUUIDs = []string{"machine-0-uuid", "machine-1-uuid"}
	netNodeUUIDs = []string{"net-node-0-uuid", "net-node-1-uuid"}
	appNames     = []string{"app-zero", "app-one"}
)

func (s *stateSuite) SetUpTest(c *tc.C) {
	s.ModelSuite.SetUpTest(c)

	modelUUID := modeltesting.GenModelUUID(c)
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `
			INSERT INTO model (uuid, controller_uuid, name, type, cloud, cloud_type)
			VALUES (?, ?, "test", "iaas", "test-model", "ec2")
		`, modelUUID.String(), coretesting.ControllerTag.Id())
		return err
	})
	c.Assert(err, tc.ErrorIsNil)

	machineSt := machinestate.NewState(s.TxnRunnerFactory(), clock.WallClock, loggertesting.WrapCheckLog(c))
	err = machineSt.CreateMachine(c.Context(), "0", netNodeUUIDs[0], machine.UUID(machineUUIDs[0]))
	c.Assert(err, tc.ErrorIsNil)
	err = machineSt.CreateMachine(c.Context(), "1", netNodeUUIDs[1], machine.UUID(machineUUIDs[1]))
	c.Assert(err, tc.ErrorIsNil)

	s.appUUID = s.createApplicationWithRelations(c, appNames[0], "ep0", "ep1", "ep2")
	s.unitUUID, s.unitName = s.createUnit(c, netNodeUUIDs[0], appNames[0])
}

func (s *baseSuite) createApplicationWithRelations(c *tc.C, appName string, relations ...string) coreapplication.ID {
	relationsMap := map[string]charm.Relation{}
	for _, relation := range relations {
		relationsMap[relation] = charm.Relation{
			Name:  relation,
			Role:  charm.RoleRequirer,
			Scope: charm.ScopeGlobal,
		}
	}

	applicationSt := applicationstate.NewState(s.TxnRunnerFactory(), clock.WallClock, loggertesting.WrapCheckLog(c))
	appUUID, err := applicationSt.CreateApplication(c.Context(), appName, application.AddApplicationArg{
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
	}, nil)
	c.Assert(err, tc.ErrorIsNil)
	return appUUID
}

// createUnit creates a new unit in state and returns its UUID. The unit is assigned
// to the net node with uuid `netNodeUUID` and application with name `appName`.
func (s *baseSuite) createUnit(c *tc.C, netNodeUUID, appName string) (coreunit.UUID, coreunit.Name) {
	ctx := c.Context()
	applicationSt := applicationstate.NewState(s.TxnRunnerFactory(), clock.WallClock, loggertesting.WrapCheckLog(c))

	appID, err := applicationSt.GetApplicationIDByName(ctx, appName)
	c.Assert(err, tc.ErrorIsNil)

	// Ensure that we place the unit on the same machine as the net node.
	var machineName machine.Name
	err = s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		err := tx.QueryRowContext(ctx, "SELECT name FROM machine WHERE net_node_uuid = ?", netNodeUUID).Scan(&machineName)
		return err
	})
	c.Assert(err, tc.ErrorIsNil)

	unitNames, err := applicationSt.AddIAASUnits(ctx, appID, application.AddUnitArg{
		Placement: deployment.Placement{
			Type:      deployment.PlacementTypeMachine,
			Directive: machineName.String(),
		},
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(unitNames, tc.HasLen, 1)
	unitName := unitNames[0]
	s.unitCount++

	var unitUUID coreunit.UUID
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

func (s *stateSuite) initialiseOpenPort(c *tc.C, st *State) {
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

func (s *stateSuite) TestGetUnitOpenedPortsBlankDB(c *tc.C) {
	st := NewState(s.TxnRunnerFactory())
	ctx := c.Context()

	groupedPortRanges, err := st.GetUnitOpenedPorts(ctx, s.unitUUID)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(groupedPortRanges, tc.HasLen, 0)

	groupedPortRanges, err = st.GetUnitOpenedPorts(ctx, "non-existent")
	c.Assert(err, tc.ErrorIsNil)
	c.Check(groupedPortRanges, tc.HasLen, 0)
}

func (s *stateSuite) TestGetUnitOpenedPorts(c *tc.C) {
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
}

func (s *stateSuite) TestGetAllOpenedPortsBlankDB(c *tc.C) {
	st := NewState(s.TxnRunnerFactory())
	ctx := c.Context()

	groupedPortRanges, err := st.GetAllOpenedPorts(ctx)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(groupedPortRanges, tc.HasLen, 0)
}

func (s *stateSuite) TestGetAllOpenedPorts(c *tc.C) {
	st := NewState(s.TxnRunnerFactory())
	ctx := c.Context()
	s.initialiseOpenPort(c, st)

	_ = s.createApplicationWithRelations(c, appNames[1], "ep0", "ep1", "ep2")
	unit1UUID, unit1Name := s.createUnit(c, netNodeUUIDs[1], appNames[1])
	err := st.UpdateUnitPorts(ctx, unit1UUID, network.GroupedPortRanges{
		"ep0": {
			{Protocol: "tcp", FromPort: 443, ToPort: 443},
			{Protocol: "udp", FromPort: 2000, ToPort: 2500},
		},
		"ep1": {
			{Protocol: "udp", FromPort: 2000, ToPort: 2500},
		},
	}, network.GroupedPortRanges{})
	c.Assert(err, tc.ErrorIsNil)

	groupedPortRanges, err := st.GetAllOpenedPorts(ctx)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(groupedPortRanges, tc.HasLen, 2)

	c.Check(groupedPortRanges[s.unitName], tc.HasLen, 3)
	c.Check(groupedPortRanges[s.unitName][0], tc.Equals, network.PortRange{Protocol: "tcp", FromPort: 80, ToPort: 80})
	c.Check(groupedPortRanges[s.unitName][1], tc.Equals, network.PortRange{Protocol: "tcp", FromPort: 8080, ToPort: 8080})
	c.Check(groupedPortRanges[s.unitName][2], tc.Equals, network.PortRange{Protocol: "udp", FromPort: 1000, ToPort: 1500})

	c.Check(groupedPortRanges[unit1Name], tc.HasLen, 2)
	c.Check(groupedPortRanges[unit1Name][0], tc.Equals, network.PortRange{Protocol: "tcp", FromPort: 443, ToPort: 443})
	c.Check(groupedPortRanges[unit1Name][1], tc.Equals, network.PortRange{Protocol: "udp", FromPort: 2000, ToPort: 2500})
}

func (s *stateSuite) TestGetMachineOpenedPortsBlankDB(c *tc.C) {
	st := NewState(s.TxnRunnerFactory())
	ctx := c.Context()

	machineGroupedPortRanges, err := st.GetMachineOpenedPorts(ctx, machineUUIDs[0])
	c.Assert(err, tc.ErrorIsNil)
	c.Check(machineGroupedPortRanges, tc.HasLen, 0)

	machineGroupedPortRanges, err = st.GetMachineOpenedPorts(ctx, "non-existent")
	c.Assert(err, tc.ErrorIsNil)
	c.Check(machineGroupedPortRanges, tc.HasLen, 0)
}

func (s *stateSuite) TestGetMachineOpenedPorts(c *tc.C) {
	st := NewState(s.TxnRunnerFactory())
	ctx := c.Context()
	s.initialiseOpenPort(c, st)

	machineGroupedPortRanges, err := st.GetMachineOpenedPorts(ctx, machineUUIDs[0])
	c.Assert(err, tc.ErrorIsNil)
	c.Check(machineGroupedPortRanges, tc.HasLen, 1)

	unit0PortRanges, ok := machineGroupedPortRanges[s.unitName]
	c.Assert(ok, tc.IsTrue)
	c.Check(unit0PortRanges, tc.HasLen, 2)

	c.Check(unit0PortRanges["ep0"], tc.HasLen, 2)
	c.Check(unit0PortRanges["ep0"][0], tc.DeepEquals, network.PortRange{Protocol: "tcp", FromPort: 80, ToPort: 80})
	c.Check(unit0PortRanges["ep0"][1], tc.DeepEquals, network.PortRange{Protocol: "udp", FromPort: 1000, ToPort: 1500})

	c.Check(unit0PortRanges["ep1"], tc.HasLen, 1)
	c.Check(unit0PortRanges["ep1"][0], tc.DeepEquals, network.PortRange{Protocol: "tcp", FromPort: 8080, ToPort: 8080})
}

func (s *stateSuite) TestGetMachineOpenedPortsAcrossTwoUnits(c *tc.C) {
	st := NewState(s.TxnRunnerFactory())
	ctx := c.Context()
	s.initialiseOpenPort(c, st)

	unit1UUID, unit1Name := s.createUnit(c, netNodeUUIDs[0], appNames[0])
	err := st.UpdateUnitPorts(ctx, unit1UUID, network.GroupedPortRanges{
		"ep0": {
			{Protocol: "tcp", FromPort: 443, ToPort: 443},
			{Protocol: "udp", FromPort: 2000, ToPort: 2500},
		},
	}, network.GroupedPortRanges{})
	c.Assert(err, tc.ErrorIsNil)

	machineGroupedPortRanges, err := st.GetMachineOpenedPorts(ctx, machineUUIDs[0])
	c.Assert(err, tc.ErrorIsNil)
	c.Check(machineGroupedPortRanges, tc.HasLen, 2)

	unit0PortRanges, ok := machineGroupedPortRanges[s.unitName]
	c.Assert(ok, tc.IsTrue)
	c.Check(unit0PortRanges, tc.HasLen, 2)

	c.Check(unit0PortRanges["ep0"], tc.HasLen, 2)
	c.Check(unit0PortRanges["ep0"][0], tc.DeepEquals, network.PortRange{Protocol: "tcp", FromPort: 80, ToPort: 80})
	c.Check(unit0PortRanges["ep0"][1], tc.DeepEquals, network.PortRange{Protocol: "udp", FromPort: 1000, ToPort: 1500})

	c.Check(unit0PortRanges["ep1"], tc.HasLen, 1)
	c.Check(unit0PortRanges["ep1"][0], tc.DeepEquals, network.PortRange{Protocol: "tcp", FromPort: 8080, ToPort: 8080})

	unit1PortRanges, ok := machineGroupedPortRanges[unit1Name]
	c.Assert(ok, tc.IsTrue)
	c.Check(unit1PortRanges, tc.HasLen, 1)

	c.Check(unit1PortRanges["ep0"], tc.HasLen, 2)
	c.Check(unit1PortRanges["ep0"][0], tc.DeepEquals, network.PortRange{Protocol: "tcp", FromPort: 443, ToPort: 443})
	c.Check(unit1PortRanges["ep0"][1], tc.DeepEquals, network.PortRange{Protocol: "udp", FromPort: 2000, ToPort: 2500})
}

func (s *stateSuite) TestGetMachineOpenedPortsAcrossTwoUnitsDifferentMachines(c *tc.C) {
	st := NewState(s.TxnRunnerFactory())
	ctx := c.Context()
	s.initialiseOpenPort(c, st)

	unit1UUID, unit1Name := s.createUnit(c, netNodeUUIDs[1], appNames[0])
	err := st.UpdateUnitPorts(ctx, unit1UUID, network.GroupedPortRanges{
		"ep0": {
			{Protocol: "tcp", FromPort: 443, ToPort: 443},
			{Protocol: "udp", FromPort: 2000, ToPort: 2500},
		},
	}, network.GroupedPortRanges{})
	c.Assert(err, tc.ErrorIsNil)

	machineGroupedPortRanges, err := st.GetMachineOpenedPorts(ctx, machineUUIDs[0])
	c.Assert(err, tc.ErrorIsNil)
	c.Check(machineGroupedPortRanges, tc.HasLen, 1)

	unit0PortRanges, ok := machineGroupedPortRanges[s.unitName]
	c.Assert(ok, tc.IsTrue)
	c.Check(unit0PortRanges, tc.HasLen, 2)

	c.Check(unit0PortRanges["ep0"], tc.HasLen, 2)
	c.Check(unit0PortRanges["ep0"][0], tc.DeepEquals, network.PortRange{Protocol: "tcp", FromPort: 80, ToPort: 80})
	c.Check(unit0PortRanges["ep0"][1], tc.DeepEquals, network.PortRange{Protocol: "udp", FromPort: 1000, ToPort: 1500})

	c.Check(unit0PortRanges["ep1"], tc.HasLen, 1)
	c.Check(unit0PortRanges["ep1"][0], tc.DeepEquals, network.PortRange{Protocol: "tcp", FromPort: 8080, ToPort: 8080})

	machineGroupedPortRanges, err = st.GetMachineOpenedPorts(ctx, machineUUIDs[1])
	c.Assert(err, tc.ErrorIsNil)
	c.Check(machineGroupedPortRanges, tc.HasLen, 1)

	unit1PortRanges, ok := machineGroupedPortRanges[unit1Name]
	c.Assert(ok, tc.IsTrue)
	c.Check(unit1PortRanges, tc.HasLen, 1)

	c.Check(unit1PortRanges["ep0"], tc.HasLen, 2)
	c.Check(unit1PortRanges["ep0"][0], tc.DeepEquals, network.PortRange{Protocol: "tcp", FromPort: 443, ToPort: 443})
	c.Check(unit1PortRanges["ep0"][1], tc.DeepEquals, network.PortRange{Protocol: "udp", FromPort: 2000, ToPort: 2500})
}

func (s *stateSuite) TestGetApplicationOpenedPortsBlankDB(c *tc.C) {
	st := NewState(s.TxnRunnerFactory())
	ctx := c.Context()

	unitEndpointPortRanges, err := st.GetApplicationOpenedPorts(ctx, "non-existent")
	c.Assert(err, tc.ErrorIsNil)
	c.Check(unitEndpointPortRanges, tc.HasLen, 0)
}

func (s *stateSuite) TestGetApplicationOpenedPorts(c *tc.C) {
	st := NewState(s.TxnRunnerFactory())
	ctx := c.Context()
	s.initialiseOpenPort(c, st)

	expect := port.UnitEndpointPortRanges{
		{Endpoint: "ep0", UnitName: s.unitName, PortRange: network.PortRange{Protocol: "tcp", FromPort: 80, ToPort: 80}},
		{Endpoint: "ep0", UnitName: s.unitName, PortRange: network.PortRange{Protocol: "udp", FromPort: 1000, ToPort: 1500}},
		{Endpoint: "ep1", UnitName: s.unitName, PortRange: network.PortRange{Protocol: "tcp", FromPort: 8080, ToPort: 8080}},
	}
	port.SortUnitEndpointPortRanges(expect)

	unitEndpointPortRanges, err := st.GetApplicationOpenedPorts(ctx, s.appUUID)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(unitEndpointPortRanges, tc.HasLen, 3)
	c.Check(unitEndpointPortRanges, tc.DeepEquals, expect)
}

func (s *stateSuite) TestGetApplicationOpenedPortsAcrossTwoUnits(c *tc.C) {
	st := NewState(s.TxnRunnerFactory())
	ctx := c.Context()
	s.initialiseOpenPort(c, st)

	unit1UUID, unit1Name := s.createUnit(c, netNodeUUIDs[1], appNames[0])
	err := st.UpdateUnitPorts(ctx, unit1UUID, network.GroupedPortRanges{
		"ep0": {
			{Protocol: "tcp", FromPort: 443, ToPort: 443},
			{Protocol: "udp", FromPort: 2000, ToPort: 2500},
		},
	}, network.GroupedPortRanges{})
	c.Assert(err, tc.ErrorIsNil)

	expect := port.UnitEndpointPortRanges{
		{Endpoint: "ep0", UnitName: s.unitName, PortRange: network.PortRange{Protocol: "tcp", FromPort: 80, ToPort: 80}},
		{Endpoint: "ep0", UnitName: s.unitName, PortRange: network.PortRange{Protocol: "udp", FromPort: 1000, ToPort: 1500}},
		{Endpoint: "ep1", UnitName: s.unitName, PortRange: network.PortRange{Protocol: "tcp", FromPort: 8080, ToPort: 8080}},
		{Endpoint: "ep0", UnitName: unit1Name, PortRange: network.PortRange{Protocol: "tcp", FromPort: 443, ToPort: 443}},
		{Endpoint: "ep0", UnitName: unit1Name, PortRange: network.PortRange{Protocol: "udp", FromPort: 2000, ToPort: 2500}},
	}
	port.SortUnitEndpointPortRanges(expect)

	unitEndpointPortRanges, err := st.GetApplicationOpenedPorts(ctx, s.appUUID)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(unitEndpointPortRanges, tc.HasLen, 5)
	c.Check(unitEndpointPortRanges, tc.DeepEquals, expect)
}

func (s *stateSuite) TestGetApplicationOpenedPortsAcrossTwoUnitsDifferentApplications(c *tc.C) {
	st := NewState(s.TxnRunnerFactory())
	ctx := c.Context()
	s.initialiseOpenPort(c, st)

	app1UUID := s.createApplicationWithRelations(c, appNames[1], "ep0", "ep1", "ep2")
	unit1UUID, unit1Name := s.createUnit(c, netNodeUUIDs[1], appNames[1])
	err := st.UpdateUnitPorts(ctx, unit1UUID, network.GroupedPortRanges{
		"ep0": {
			{Protocol: "tcp", FromPort: 443, ToPort: 443},
			{Protocol: "udp", FromPort: 2000, ToPort: 2500},
		},
	}, network.GroupedPortRanges{})
	c.Assert(err, tc.ErrorIsNil)

	expect := port.UnitEndpointPortRanges{
		{Endpoint: "ep0", UnitName: s.unitName, PortRange: network.PortRange{Protocol: "tcp", FromPort: 80, ToPort: 80}},
		{Endpoint: "ep0", UnitName: s.unitName, PortRange: network.PortRange{Protocol: "udp", FromPort: 1000, ToPort: 1500}},
		{Endpoint: "ep1", UnitName: s.unitName, PortRange: network.PortRange{Protocol: "tcp", FromPort: 8080, ToPort: 8080}},
	}
	port.SortUnitEndpointPortRanges(expect)

	unitEndpointPortRanges, err := st.GetApplicationOpenedPorts(ctx, s.appUUID)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(unitEndpointPortRanges, tc.HasLen, 3)
	c.Check(unitEndpointPortRanges, tc.DeepEquals, expect)

	expect = port.UnitEndpointPortRanges{
		{Endpoint: "ep0", UnitName: unit1Name, PortRange: network.PortRange{Protocol: "tcp", FromPort: 443, ToPort: 443}},
		{Endpoint: "ep0", UnitName: unit1Name, PortRange: network.PortRange{Protocol: "udp", FromPort: 2000, ToPort: 2500}},
	}

	unitEndpointPortRanges, err = st.GetApplicationOpenedPorts(ctx, app1UUID)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(unitEndpointPortRanges, tc.HasLen, 2)
	c.Check(unitEndpointPortRanges, tc.DeepEquals, expect)
}

func (s *stateSuite) TestGetUnitUUID(c *tc.C) {
	st := NewState(s.TxnRunnerFactory())
	ctx := c.Context()

	unitUUID, err := st.GetUnitUUID(ctx, s.unitName)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(unitUUID, tc.Equals, s.unitUUID)
}

func (s *stateSuite) TestGetUnitUUIDNotFound(c *tc.C) {
	st := NewState(s.TxnRunnerFactory())
	ctx := c.Context()

	_, err := st.GetUnitUUID(ctx, "blah")
	c.Assert(err, tc.ErrorIs, porterrors.UnitNotFound)
}
