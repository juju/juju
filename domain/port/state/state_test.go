// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"database/sql"

	"github.com/juju/clock"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

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

var _ = gc.Suite(&stateSuite{})

var (
	machineUUIDs = []string{"machine-0-uuid", "machine-1-uuid"}
	netNodeUUIDs = []string{"net-node-0-uuid", "net-node-1-uuid"}
	appNames     = []string{"app-zero", "app-one"}
)

func (s *stateSuite) SetUpTest(c *gc.C) {
	s.ModelSuite.SetUpTest(c)

	modelUUID := modeltesting.GenModelUUID(c)
	err := s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `
			INSERT INTO model (uuid, controller_uuid, name, type, cloud, cloud_type)
			VALUES (?, ?, "test", "iaas", "test-model", "ec2")
		`, modelUUID.String(), coretesting.ControllerTag.Id())
		return err
	})
	c.Assert(err, jc.ErrorIsNil)

	machineSt := machinestate.NewState(s.TxnRunnerFactory(), clock.WallClock, loggertesting.WrapCheckLog(c))
	err = machineSt.CreateMachine(context.Background(), "0", netNodeUUIDs[0], machine.UUID(machineUUIDs[0]))
	c.Assert(err, jc.ErrorIsNil)
	err = machineSt.CreateMachine(context.Background(), "1", netNodeUUIDs[1], machine.UUID(machineUUIDs[1]))
	c.Assert(err, jc.ErrorIsNil)

	s.appUUID = s.createApplicationWithRelations(c, appNames[0], "ep0", "ep1", "ep2")
	s.unitUUID, s.unitName = s.createUnit(c, netNodeUUIDs[0], appNames[0])
}

func (s *baseSuite) createApplicationWithRelations(c *gc.C, appName string, relations ...string) coreapplication.ID {
	relationsMap := map[string]charm.Relation{}
	for _, relation := range relations {
		relationsMap[relation] = charm.Relation{
			Name:  relation,
			Role:  charm.RoleRequirer,
			Scope: charm.ScopeGlobal,
		}
	}

	applicationSt := applicationstate.NewState(s.TxnRunnerFactory(), clock.WallClock, loggertesting.WrapCheckLog(c))
	appUUID, err := applicationSt.CreateApplication(context.Background(), appName, application.AddApplicationArg{
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
	c.Assert(err, jc.ErrorIsNil)
	return appUUID
}

// createUnit creates a new unit in state and returns its UUID. The unit is assigned
// to the net node with uuid `netNodeUUID` and application with name `appName`.
func (s *baseSuite) createUnit(c *gc.C, netNodeUUID, appName string) (coreunit.UUID, coreunit.Name) {
	ctx := context.Background()
	applicationSt := applicationstate.NewState(s.TxnRunnerFactory(), clock.WallClock, loggertesting.WrapCheckLog(c))

	appID, err := applicationSt.GetApplicationIDByName(ctx, appName)
	c.Assert(err, jc.ErrorIsNil)

	charmUUID, err := applicationSt.GetCharmIDByApplicationName(ctx, appName)
	c.Assert(err, jc.ErrorIsNil)

	// Ensure that we place the unit on the same machine as the net node.
	var machineName machine.Name
	err = s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		err := tx.QueryRowContext(ctx, "SELECT name FROM machine WHERE net_node_uuid = ?", netNodeUUID).Scan(&machineName)
		return err
	})
	c.Assert(err, jc.ErrorIsNil)

	unitNames, err := applicationSt.AddIAASUnits(ctx, appID, charmUUID, application.AddUnitArg{
		Placement: deployment.Placement{
			Type:      deployment.PlacementTypeMachine,
			Directive: machineName.String(),
		},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(unitNames, gc.HasLen, 1)
	unitName := unitNames[0]
	s.unitCount++

	var unitUUID coreunit.UUID
	err = s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		err := tx.QueryRowContext(ctx, "SELECT uuid FROM unit WHERE name = ?", unitName).Scan(&unitUUID)
		if err != nil {
			return err
		}

		return nil
	})
	c.Assert(err, jc.ErrorIsNil)
	return unitUUID, unitName
}

func (s *stateSuite) initialiseOpenPort(c *gc.C, st *State) {
	ctx := context.Background()
	err := st.UpdateUnitPorts(ctx, s.unitUUID, network.GroupedPortRanges{
		"ep0": {
			{Protocol: "tcp", FromPort: 80, ToPort: 80},
			{Protocol: "udp", FromPort: 1000, ToPort: 1500},
		},
		"ep1": {
			{Protocol: "tcp", FromPort: 8080, ToPort: 8080},
		},
	}, network.GroupedPortRanges{})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *stateSuite) TestGetUnitOpenedPortsBlankDB(c *gc.C) {
	st := NewState(s.TxnRunnerFactory())
	ctx := context.Background()

	groupedPortRanges, err := st.GetUnitOpenedPorts(ctx, s.unitUUID)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(groupedPortRanges, gc.HasLen, 0)

	groupedPortRanges, err = st.GetUnitOpenedPorts(ctx, "non-existent")
	c.Assert(err, jc.ErrorIsNil)
	c.Check(groupedPortRanges, gc.HasLen, 0)
}

func (s *stateSuite) TestGetUnitOpenedPorts(c *gc.C) {
	st := NewState(s.TxnRunnerFactory())
	ctx := context.Background()
	s.initialiseOpenPort(c, st)

	groupedPortRanges, err := st.GetUnitOpenedPorts(ctx, s.unitUUID)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(groupedPortRanges, gc.HasLen, 2)

	c.Check(groupedPortRanges["ep0"], gc.HasLen, 2)
	c.Check(groupedPortRanges["ep0"][0], jc.DeepEquals, network.PortRange{Protocol: "tcp", FromPort: 80, ToPort: 80})
	c.Check(groupedPortRanges["ep0"][1], jc.DeepEquals, network.PortRange{Protocol: "udp", FromPort: 1000, ToPort: 1500})

	c.Check(groupedPortRanges["ep1"], gc.HasLen, 1)
	c.Check(groupedPortRanges["ep1"][0], jc.DeepEquals, network.PortRange{Protocol: "tcp", FromPort: 8080, ToPort: 8080})
}

func (s *stateSuite) TestGetAllOpenedPortsBlankDB(c *gc.C) {
	st := NewState(s.TxnRunnerFactory())
	ctx := context.Background()

	groupedPortRanges, err := st.GetAllOpenedPorts(ctx)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(groupedPortRanges, gc.HasLen, 0)
}

func (s *stateSuite) TestGetAllOpenedPorts(c *gc.C) {
	st := NewState(s.TxnRunnerFactory())
	ctx := context.Background()
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
	c.Assert(err, jc.ErrorIsNil)

	groupedPortRanges, err := st.GetAllOpenedPorts(ctx)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(groupedPortRanges, gc.HasLen, 2)

	c.Check(groupedPortRanges[s.unitName], gc.HasLen, 3)
	c.Check(groupedPortRanges[s.unitName][0], gc.Equals, network.PortRange{Protocol: "tcp", FromPort: 80, ToPort: 80})
	c.Check(groupedPortRanges[s.unitName][1], gc.Equals, network.PortRange{Protocol: "tcp", FromPort: 8080, ToPort: 8080})
	c.Check(groupedPortRanges[s.unitName][2], gc.Equals, network.PortRange{Protocol: "udp", FromPort: 1000, ToPort: 1500})

	c.Check(groupedPortRanges[unit1Name], gc.HasLen, 2)
	c.Check(groupedPortRanges[unit1Name][0], gc.Equals, network.PortRange{Protocol: "tcp", FromPort: 443, ToPort: 443})
	c.Check(groupedPortRanges[unit1Name][1], gc.Equals, network.PortRange{Protocol: "udp", FromPort: 2000, ToPort: 2500})
}

func (s *stateSuite) TestGetMachineOpenedPortsBlankDB(c *gc.C) {
	st := NewState(s.TxnRunnerFactory())
	ctx := context.Background()

	machineGroupedPortRanges, err := st.GetMachineOpenedPorts(ctx, machineUUIDs[0])
	c.Assert(err, jc.ErrorIsNil)
	c.Check(machineGroupedPortRanges, gc.HasLen, 0)

	machineGroupedPortRanges, err = st.GetMachineOpenedPorts(ctx, "non-existent")
	c.Assert(err, jc.ErrorIsNil)
	c.Check(machineGroupedPortRanges, gc.HasLen, 0)
}

func (s *stateSuite) TestGetMachineOpenedPorts(c *gc.C) {
	st := NewState(s.TxnRunnerFactory())
	ctx := context.Background()
	s.initialiseOpenPort(c, st)

	machineGroupedPortRanges, err := st.GetMachineOpenedPorts(ctx, machineUUIDs[0])
	c.Assert(err, jc.ErrorIsNil)
	c.Check(machineGroupedPortRanges, gc.HasLen, 1)

	unit0PortRanges, ok := machineGroupedPortRanges[s.unitName]
	c.Assert(ok, jc.IsTrue)
	c.Check(unit0PortRanges, gc.HasLen, 2)

	c.Check(unit0PortRanges["ep0"], gc.HasLen, 2)
	c.Check(unit0PortRanges["ep0"][0], jc.DeepEquals, network.PortRange{Protocol: "tcp", FromPort: 80, ToPort: 80})
	c.Check(unit0PortRanges["ep0"][1], jc.DeepEquals, network.PortRange{Protocol: "udp", FromPort: 1000, ToPort: 1500})

	c.Check(unit0PortRanges["ep1"], gc.HasLen, 1)
	c.Check(unit0PortRanges["ep1"][0], jc.DeepEquals, network.PortRange{Protocol: "tcp", FromPort: 8080, ToPort: 8080})
}

func (s *stateSuite) TestGetMachineOpenedPortsAcrossTwoUnits(c *gc.C) {
	st := NewState(s.TxnRunnerFactory())
	ctx := context.Background()
	s.initialiseOpenPort(c, st)

	unit1UUID, unit1Name := s.createUnit(c, netNodeUUIDs[0], appNames[0])
	err := st.UpdateUnitPorts(ctx, unit1UUID, network.GroupedPortRanges{
		"ep0": {
			{Protocol: "tcp", FromPort: 443, ToPort: 443},
			{Protocol: "udp", FromPort: 2000, ToPort: 2500},
		},
	}, network.GroupedPortRanges{})
	c.Assert(err, jc.ErrorIsNil)

	machineGroupedPortRanges, err := st.GetMachineOpenedPorts(ctx, machineUUIDs[0])
	c.Assert(err, jc.ErrorIsNil)
	c.Check(machineGroupedPortRanges, gc.HasLen, 2)

	unit0PortRanges, ok := machineGroupedPortRanges[s.unitName]
	c.Assert(ok, jc.IsTrue)
	c.Check(unit0PortRanges, gc.HasLen, 2)

	c.Check(unit0PortRanges["ep0"], gc.HasLen, 2)
	c.Check(unit0PortRanges["ep0"][0], jc.DeepEquals, network.PortRange{Protocol: "tcp", FromPort: 80, ToPort: 80})
	c.Check(unit0PortRanges["ep0"][1], jc.DeepEquals, network.PortRange{Protocol: "udp", FromPort: 1000, ToPort: 1500})

	c.Check(unit0PortRanges["ep1"], gc.HasLen, 1)
	c.Check(unit0PortRanges["ep1"][0], jc.DeepEquals, network.PortRange{Protocol: "tcp", FromPort: 8080, ToPort: 8080})

	unit1PortRanges, ok := machineGroupedPortRanges[unit1Name]
	c.Assert(ok, jc.IsTrue)
	c.Check(unit1PortRanges, gc.HasLen, 1)

	c.Check(unit1PortRanges["ep0"], gc.HasLen, 2)
	c.Check(unit1PortRanges["ep0"][0], jc.DeepEquals, network.PortRange{Protocol: "tcp", FromPort: 443, ToPort: 443})
	c.Check(unit1PortRanges["ep0"][1], jc.DeepEquals, network.PortRange{Protocol: "udp", FromPort: 2000, ToPort: 2500})
}

func (s *stateSuite) TestGetMachineOpenedPortsAcrossTwoUnitsDifferentMachines(c *gc.C) {
	st := NewState(s.TxnRunnerFactory())
	ctx := context.Background()
	s.initialiseOpenPort(c, st)

	unit1UUID, unit1Name := s.createUnit(c, netNodeUUIDs[1], appNames[0])
	err := st.UpdateUnitPorts(ctx, unit1UUID, network.GroupedPortRanges{
		"ep0": {
			{Protocol: "tcp", FromPort: 443, ToPort: 443},
			{Protocol: "udp", FromPort: 2000, ToPort: 2500},
		},
	}, network.GroupedPortRanges{})
	c.Assert(err, jc.ErrorIsNil)

	machineGroupedPortRanges, err := st.GetMachineOpenedPorts(ctx, machineUUIDs[0])
	c.Assert(err, jc.ErrorIsNil)
	c.Check(machineGroupedPortRanges, gc.HasLen, 1)

	unit0PortRanges, ok := machineGroupedPortRanges[s.unitName]
	c.Assert(ok, jc.IsTrue)
	c.Check(unit0PortRanges, gc.HasLen, 2)

	c.Check(unit0PortRanges["ep0"], gc.HasLen, 2)
	c.Check(unit0PortRanges["ep0"][0], jc.DeepEquals, network.PortRange{Protocol: "tcp", FromPort: 80, ToPort: 80})
	c.Check(unit0PortRanges["ep0"][1], jc.DeepEquals, network.PortRange{Protocol: "udp", FromPort: 1000, ToPort: 1500})

	c.Check(unit0PortRanges["ep1"], gc.HasLen, 1)
	c.Check(unit0PortRanges["ep1"][0], jc.DeepEquals, network.PortRange{Protocol: "tcp", FromPort: 8080, ToPort: 8080})

	machineGroupedPortRanges, err = st.GetMachineOpenedPorts(ctx, machineUUIDs[1])
	c.Assert(err, jc.ErrorIsNil)
	c.Check(machineGroupedPortRanges, gc.HasLen, 1)

	unit1PortRanges, ok := machineGroupedPortRanges[unit1Name]
	c.Assert(ok, jc.IsTrue)
	c.Check(unit1PortRanges, gc.HasLen, 1)

	c.Check(unit1PortRanges["ep0"], gc.HasLen, 2)
	c.Check(unit1PortRanges["ep0"][0], jc.DeepEquals, network.PortRange{Protocol: "tcp", FromPort: 443, ToPort: 443})
	c.Check(unit1PortRanges["ep0"][1], jc.DeepEquals, network.PortRange{Protocol: "udp", FromPort: 2000, ToPort: 2500})
}

func (s *stateSuite) TestGetApplicationOpenedPortsBlankDB(c *gc.C) {
	st := NewState(s.TxnRunnerFactory())
	ctx := context.Background()

	unitEndpointPortRanges, err := st.GetApplicationOpenedPorts(ctx, "non-existent")
	c.Assert(err, jc.ErrorIsNil)
	c.Check(unitEndpointPortRanges, gc.HasLen, 0)
}

func (s *stateSuite) TestGetApplicationOpenedPorts(c *gc.C) {
	st := NewState(s.TxnRunnerFactory())
	ctx := context.Background()
	s.initialiseOpenPort(c, st)

	expect := port.UnitEndpointPortRanges{
		{Endpoint: "ep0", UnitName: s.unitName, PortRange: network.PortRange{Protocol: "tcp", FromPort: 80, ToPort: 80}},
		{Endpoint: "ep0", UnitName: s.unitName, PortRange: network.PortRange{Protocol: "udp", FromPort: 1000, ToPort: 1500}},
		{Endpoint: "ep1", UnitName: s.unitName, PortRange: network.PortRange{Protocol: "tcp", FromPort: 8080, ToPort: 8080}},
	}
	port.SortUnitEndpointPortRanges(expect)

	unitEndpointPortRanges, err := st.GetApplicationOpenedPorts(ctx, s.appUUID)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(unitEndpointPortRanges, gc.HasLen, 3)
	c.Check(unitEndpointPortRanges, jc.DeepEquals, expect)
}

func (s *stateSuite) TestGetApplicationOpenedPortsAcrossTwoUnits(c *gc.C) {
	st := NewState(s.TxnRunnerFactory())
	ctx := context.Background()
	s.initialiseOpenPort(c, st)

	unit1UUID, unit1Name := s.createUnit(c, netNodeUUIDs[1], appNames[0])
	err := st.UpdateUnitPorts(ctx, unit1UUID, network.GroupedPortRanges{
		"ep0": {
			{Protocol: "tcp", FromPort: 443, ToPort: 443},
			{Protocol: "udp", FromPort: 2000, ToPort: 2500},
		},
	}, network.GroupedPortRanges{})
	c.Assert(err, jc.ErrorIsNil)

	expect := port.UnitEndpointPortRanges{
		{Endpoint: "ep0", UnitName: s.unitName, PortRange: network.PortRange{Protocol: "tcp", FromPort: 80, ToPort: 80}},
		{Endpoint: "ep0", UnitName: s.unitName, PortRange: network.PortRange{Protocol: "udp", FromPort: 1000, ToPort: 1500}},
		{Endpoint: "ep1", UnitName: s.unitName, PortRange: network.PortRange{Protocol: "tcp", FromPort: 8080, ToPort: 8080}},
		{Endpoint: "ep0", UnitName: unit1Name, PortRange: network.PortRange{Protocol: "tcp", FromPort: 443, ToPort: 443}},
		{Endpoint: "ep0", UnitName: unit1Name, PortRange: network.PortRange{Protocol: "udp", FromPort: 2000, ToPort: 2500}},
	}
	port.SortUnitEndpointPortRanges(expect)

	unitEndpointPortRanges, err := st.GetApplicationOpenedPorts(ctx, s.appUUID)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(unitEndpointPortRanges, gc.HasLen, 5)
	c.Check(unitEndpointPortRanges, jc.DeepEquals, expect)
}

func (s *stateSuite) TestGetApplicationOpenedPortsAcrossTwoUnitsDifferentApplications(c *gc.C) {
	st := NewState(s.TxnRunnerFactory())
	ctx := context.Background()
	s.initialiseOpenPort(c, st)

	app1UUID := s.createApplicationWithRelations(c, appNames[1], "ep0", "ep1", "ep2")
	unit1UUID, unit1Name := s.createUnit(c, netNodeUUIDs[1], appNames[1])
	err := st.UpdateUnitPorts(ctx, unit1UUID, network.GroupedPortRanges{
		"ep0": {
			{Protocol: "tcp", FromPort: 443, ToPort: 443},
			{Protocol: "udp", FromPort: 2000, ToPort: 2500},
		},
	}, network.GroupedPortRanges{})
	c.Assert(err, jc.ErrorIsNil)

	expect := port.UnitEndpointPortRanges{
		{Endpoint: "ep0", UnitName: s.unitName, PortRange: network.PortRange{Protocol: "tcp", FromPort: 80, ToPort: 80}},
		{Endpoint: "ep0", UnitName: s.unitName, PortRange: network.PortRange{Protocol: "udp", FromPort: 1000, ToPort: 1500}},
		{Endpoint: "ep1", UnitName: s.unitName, PortRange: network.PortRange{Protocol: "tcp", FromPort: 8080, ToPort: 8080}},
	}
	port.SortUnitEndpointPortRanges(expect)

	unitEndpointPortRanges, err := st.GetApplicationOpenedPorts(ctx, s.appUUID)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(unitEndpointPortRanges, gc.HasLen, 3)
	c.Check(unitEndpointPortRanges, jc.DeepEquals, expect)

	expect = port.UnitEndpointPortRanges{
		{Endpoint: "ep0", UnitName: unit1Name, PortRange: network.PortRange{Protocol: "tcp", FromPort: 443, ToPort: 443}},
		{Endpoint: "ep0", UnitName: unit1Name, PortRange: network.PortRange{Protocol: "udp", FromPort: 2000, ToPort: 2500}},
	}

	unitEndpointPortRanges, err = st.GetApplicationOpenedPorts(ctx, app1UUID)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(unitEndpointPortRanges, gc.HasLen, 2)
	c.Check(unitEndpointPortRanges, jc.DeepEquals, expect)
}

func (s *stateSuite) TestGetUnitUUID(c *gc.C) {
	st := NewState(s.TxnRunnerFactory())
	ctx := context.Background()

	unitUUID, err := st.GetUnitUUID(ctx, s.unitName)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(unitUUID, gc.Equals, s.unitUUID)
}

func (s *stateSuite) TestGetUnitUUIDNotFound(c *gc.C) {
	st := NewState(s.TxnRunnerFactory())
	ctx := context.Background()

	_, err := st.GetUnitUUID(ctx, "blah")
	c.Assert(err, jc.ErrorIs, porterrors.UnitNotFound)
}
