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
	"github.com/juju/juju/core/network"
	coreunit "github.com/juju/juju/core/unit"
	"github.com/juju/juju/domain"
	"github.com/juju/juju/domain/application"
	"github.com/juju/juju/domain/application/charm"
	applicationstate "github.com/juju/juju/domain/application/state"
	machinestate "github.com/juju/juju/domain/machine/state"
	"github.com/juju/juju/domain/port"
	porterrors "github.com/juju/juju/domain/port/errors"
	"github.com/juju/juju/internal/changestream/testing"
	"github.com/juju/juju/internal/logger"
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

	machineSt := machinestate.NewState(s.TxnRunnerFactory(), logger.GetLogger("juju.test.machine"))
	err := machineSt.CreateMachine(context.Background(), "m", netNodeUUIDs[0], machineUUIDs[0])
	c.Assert(err, jc.ErrorIsNil)

	s.appUUID = s.createApplicationWithRelations(c, appNames[0], "ep0", "ep1", "ep2")
	s.unitUUID, s.unitName = s.createUnit(c, netNodeUUIDs[0], appNames[0])
}

func (s *baseSuite) createApplicationWithRelations(c *gc.C, appName string, relations ...string) coreapplication.ID {
	relationsMap := map[string]charm.Relation{}
	for _, relation := range relations {
		relationsMap[relation] = charm.Relation{
			Name:  relation,
			Key:   relation,
			Role:  charm.RoleRequirer,
			Scope: charm.ScopeGlobal,
		}
	}

	applicationSt := applicationstate.NewState(s.TxnRunnerFactory(), clock.WallClock, logger.GetLogger("juju.test.application"))
	var appUUID coreapplication.ID
	err := applicationSt.RunAtomic(context.Background(), func(ctx domain.AtomicContext) error {
		var err error
		appUUID, err = applicationSt.CreateApplication(ctx, appName, application.AddApplicationArg{
			Charm: charm.Charm{
				Metadata: charm.Metadata{
					Name:     appName,
					Requires: relationsMap,
				},
			},
		})
		return err
	})
	c.Assert(err, jc.ErrorIsNil)
	return appUUID
}

// createUnit creates a new unit in state and returns its UUID. The unit is assigned
// to the net node with uuid `netNodeUUID` and application with name `appName`.
func (s *baseSuite) createUnit(c *gc.C, netNodeUUID, appName string) (coreunit.UUID, coreunit.Name) {
	unitName, err := coreunit.NewNameFromParts(appName, s.unitCount)
	c.Assert(err, jc.ErrorIsNil)

	applicationSt := applicationstate.NewState(s.TxnRunnerFactory(), clock.WallClock, logger.GetLogger("juju.test.application"))
	err = applicationSt.RunAtomic(context.Background(), func(ctx domain.AtomicContext) error {
		appID, err := applicationSt.GetApplicationID(ctx, appName)
		if err != nil {
			return err
		}
		return applicationSt.AddUnits(ctx, appID, application.AddUnitArg{UnitName: unitName})
	})
	c.Assert(err, jc.ErrorIsNil)
	s.unitCount++

	var (
		unitUUID coreunit.UUID
	)
	err = s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		err := tx.QueryRowContext(ctx, "SELECT uuid FROM unit WHERE name = ?", unitName).Scan(&unitUUID)
		if err != nil {
			return err
		}

		_, err = tx.ExecContext(ctx, "INSERT INTO net_node VALUES (?) ON CONFLICT DO NOTHING", netNodeUUID)
		if err != nil {
			return err
		}

		_, err = tx.ExecContext(ctx, "UPDATE unit SET net_node_uuid = ? WHERE name = ?", netNodeUUID, unitName)
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
	err := st.RunAtomic(ctx, func(ctx domain.AtomicContext) error {
		return st.UpdateUnitPorts(ctx, s.unitUUID, network.GroupedPortRanges{
			"ep0": {
				{Protocol: "tcp", FromPort: 80, ToPort: 80},
				{Protocol: "udp", FromPort: 1000, ToPort: 1500},
			},
			"ep1": {
				{Protocol: "tcp", FromPort: 8080, ToPort: 8080},
			},
		}, network.GroupedPortRanges{})
	})
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
	err := st.RunAtomic(ctx, func(ctx domain.AtomicContext) error {
		return st.UpdateUnitPorts(ctx, unit1UUID, network.GroupedPortRanges{
			"ep0": {
				{Protocol: "tcp", FromPort: 443, ToPort: 443},
				{Protocol: "udp", FromPort: 2000, ToPort: 2500},
			},
			"ep1": {
				{Protocol: "udp", FromPort: 2000, ToPort: 2500},
			},
		}, network.GroupedPortRanges{})
	})
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
	err := st.RunAtomic(ctx, func(ctx domain.AtomicContext) error {
		return st.UpdateUnitPorts(ctx, unit1UUID, network.GroupedPortRanges{
			"ep0": {
				{Protocol: "tcp", FromPort: 443, ToPort: 443},
				{Protocol: "udp", FromPort: 2000, ToPort: 2500},
			},
		}, network.GroupedPortRanges{})
	})
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

	machineSt := machinestate.NewState(s.TxnRunnerFactory(), logger.GetLogger("juju.test.machine"))
	err := machineSt.CreateMachine(context.Background(), "m1", netNodeUUIDs[1], machineUUIDs[1])
	c.Assert(err, jc.ErrorIsNil)

	unit1UUID, unit1Name := s.createUnit(c, netNodeUUIDs[1], appNames[0])
	err = st.RunAtomic(ctx, func(ctx domain.AtomicContext) error {
		return st.UpdateUnitPorts(ctx, unit1UUID, network.GroupedPortRanges{
			"ep0": {
				{Protocol: "tcp", FromPort: 443, ToPort: 443},
				{Protocol: "udp", FromPort: 2000, ToPort: 2500},
			},
		}, network.GroupedPortRanges{})
	})
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
	err := st.RunAtomic(ctx, func(ctx domain.AtomicContext) error {
		return st.UpdateUnitPorts(ctx, unit1UUID, network.GroupedPortRanges{
			"ep0": {
				{Protocol: "tcp", FromPort: 443, ToPort: 443},
				{Protocol: "udp", FromPort: 2000, ToPort: 2500},
			},
		}, network.GroupedPortRanges{})
	})
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
	err := st.RunAtomic(ctx, func(ctx domain.AtomicContext) error {
		return st.UpdateUnitPorts(ctx, unit1UUID, network.GroupedPortRanges{
			"ep0": {
				{Protocol: "tcp", FromPort: 443, ToPort: 443},
				{Protocol: "udp", FromPort: 2000, ToPort: 2500},
			},
		}, network.GroupedPortRanges{})
	})
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

func (s *stateSuite) TestGetColocatedOpenedPortsSingleUnit(c *gc.C) {
	st := NewState(s.TxnRunnerFactory())
	ctx := context.Background()
	s.initialiseOpenPort(c, st)

	var opendPorts []network.PortRange
	err := st.RunAtomic(ctx, func(ctx domain.AtomicContext) error {
		var err error
		opendPorts, err = st.GetColocatedOpenedPorts(ctx, s.unitUUID)
		return err
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Check(opendPorts, gc.HasLen, 3)
	c.Check(opendPorts[0], jc.DeepEquals, network.PortRange{Protocol: "tcp", FromPort: 80, ToPort: 80})
	c.Check(opendPorts[1], jc.DeepEquals, network.PortRange{Protocol: "tcp", FromPort: 8080, ToPort: 8080})
	c.Check(opendPorts[2], jc.DeepEquals, network.PortRange{Protocol: "udp", FromPort: 1000, ToPort: 1500})
}

func (s *stateSuite) TestGetColocatedOpenedPortsMultipleUnits(c *gc.C) {
	st := NewState(s.TxnRunnerFactory())
	ctx := context.Background()
	s.initialiseOpenPort(c, st)

	unit1UUID, _ := s.createUnit(c, netNodeUUIDs[0], appNames[0])
	err := st.RunAtomic(ctx, func(ctx domain.AtomicContext) error {
		return st.UpdateUnitPorts(ctx, unit1UUID, network.GroupedPortRanges{
			"ep0": {
				{Protocol: "tcp", FromPort: 443, ToPort: 443},
				{Protocol: "udp", FromPort: 2000, ToPort: 2500},
			},
		}, network.GroupedPortRanges{})
	})
	c.Assert(err, jc.ErrorIsNil)

	var opendPorts []network.PortRange
	err = st.RunAtomic(ctx, func(ctx domain.AtomicContext) error {
		var err error
		opendPorts, err = st.GetColocatedOpenedPorts(ctx, s.unitUUID)
		return err
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Check(opendPorts, gc.HasLen, 5)
	c.Check(opendPorts[0], jc.DeepEquals, network.PortRange{Protocol: "tcp", FromPort: 80, ToPort: 80})
	c.Check(opendPorts[1], jc.DeepEquals, network.PortRange{Protocol: "tcp", FromPort: 443, ToPort: 443})
	c.Check(opendPorts[2], jc.DeepEquals, network.PortRange{Protocol: "tcp", FromPort: 8080, ToPort: 8080})
	c.Check(opendPorts[3], jc.DeepEquals, network.PortRange{Protocol: "udp", FromPort: 1000, ToPort: 1500})
	c.Check(opendPorts[4], jc.DeepEquals, network.PortRange{Protocol: "udp", FromPort: 2000, ToPort: 2500})
}

func (s *stateSuite) TestGetColocatedOpenedPortsMultipleUnitsOnNetNodes(c *gc.C) {
	st := NewState(s.TxnRunnerFactory())
	ctx := context.Background()
	s.initialiseOpenPort(c, st)

	unit1UUID, _ := s.createUnit(c, netNodeUUIDs[1], appNames[0])
	err := st.RunAtomic(ctx, func(ctx domain.AtomicContext) error {
		return st.UpdateUnitPorts(ctx, unit1UUID, network.GroupedPortRanges{
			"ep0": {
				{Protocol: "tcp", FromPort: 443, ToPort: 443},
				{Protocol: "udp", FromPort: 2000, ToPort: 2500},
			},
		}, network.GroupedPortRanges{})
	})
	c.Assert(err, jc.ErrorIsNil)

	var opendPorts []network.PortRange
	err = st.RunAtomic(ctx, func(ctx domain.AtomicContext) error {
		var err error
		opendPorts, err = st.GetColocatedOpenedPorts(ctx, s.unitUUID)
		return err
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Check(opendPorts, gc.HasLen, 3)
	c.Check(opendPorts[0], jc.DeepEquals, network.PortRange{Protocol: "tcp", FromPort: 80, ToPort: 80})
	c.Check(opendPorts[1], jc.DeepEquals, network.PortRange{Protocol: "tcp", FromPort: 8080, ToPort: 8080})
	c.Check(opendPorts[2], jc.DeepEquals, network.PortRange{Protocol: "udp", FromPort: 1000, ToPort: 1500})
}

func (s *stateSuite) TestGetEndpointOpenedPorts(c *gc.C) {
	st := NewState(s.TxnRunnerFactory())
	ctx := context.Background()
	s.initialiseOpenPort(c, st)

	err := st.RunAtomic(ctx, func(ctx domain.AtomicContext) error {
		portRanges, err := st.GetEndpointOpenedPorts(ctx, s.unitUUID, "ep0")
		c.Assert(err, jc.ErrorIsNil)
		c.Check(portRanges, gc.HasLen, 2)
		c.Check(portRanges[0], jc.DeepEquals, network.PortRange{Protocol: "tcp", FromPort: 80, ToPort: 80})
		c.Check(portRanges[1], jc.DeepEquals, network.PortRange{Protocol: "udp", FromPort: 1000, ToPort: 1500})

		portRanges, err = st.GetEndpointOpenedPorts(ctx, s.unitUUID, "ep1")
		c.Assert(err, jc.ErrorIsNil)
		c.Check(portRanges, gc.HasLen, 1)
		c.Check(portRanges[0], jc.DeepEquals, network.PortRange{Protocol: "tcp", FromPort: 8080, ToPort: 8080})

		return nil
	})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *stateSuite) TestGetEndpointOpenedPortsNonExistentEndpoint(c *gc.C) {
	st := NewState(s.TxnRunnerFactory())
	ctx := context.Background()
	s.initialiseOpenPort(c, st)

	err := st.RunAtomic(ctx, func(ctx domain.AtomicContext) error {
		portRanges, err := st.GetEndpointOpenedPorts(ctx, s.unitUUID, "ep2")
		c.Assert(err, jc.ErrorIsNil)
		c.Check(portRanges, gc.HasLen, 0)
		return nil
	})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *stateSuite) TestUpdateUnitPortsOpenPort(c *gc.C) {
	st := NewState(s.TxnRunnerFactory())
	ctx := context.Background()
	s.initialiseOpenPort(c, st)

	err := st.RunAtomic(ctx, func(ctx domain.AtomicContext) error {
		return st.UpdateUnitPorts(ctx, s.unitUUID, network.GroupedPortRanges{"ep0": {{Protocol: "tcp", FromPort: 1000, ToPort: 1500}}}, network.GroupedPortRanges{})
	})
	c.Assert(err, jc.ErrorIsNil)

	groupedPortRanges, err := st.GetUnitOpenedPorts(ctx, s.unitUUID)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(groupedPortRanges, gc.HasLen, 2)
	c.Check(groupedPortRanges["ep0"], gc.HasLen, 3)
	c.Check(groupedPortRanges["ep0"][0], jc.DeepEquals, network.PortRange{Protocol: "tcp", FromPort: 80, ToPort: 80})
	c.Check(groupedPortRanges["ep0"][1], jc.DeepEquals, network.PortRange{Protocol: "tcp", FromPort: 1000, ToPort: 1500})
	c.Check(groupedPortRanges["ep0"][2], jc.DeepEquals, network.PortRange{Protocol: "udp", FromPort: 1000, ToPort: 1500})

	c.Check(groupedPortRanges["ep1"], gc.HasLen, 1)
	c.Check(groupedPortRanges["ep1"][0], jc.DeepEquals, network.PortRange{Protocol: "tcp", FromPort: 8080, ToPort: 8080})
}

func (s *stateSuite) TestUpdateUnitPortsOpenPortWildcardEndpoint(c *gc.C) {
	st := NewState(s.TxnRunnerFactory())
	ctx := context.Background()

	err := st.RunAtomic(ctx, func(ctx domain.AtomicContext) error {
		return st.UpdateUnitPorts(ctx, s.unitUUID, network.GroupedPortRanges{
			port.WildcardEndpoint: {{Protocol: "tcp", FromPort: 1000, ToPort: 1500}},
		}, network.GroupedPortRanges{})
	})
	c.Assert(err, jc.ErrorIsNil)

	groupedPortRanges, err := st.GetUnitOpenedPorts(ctx, s.unitUUID)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(groupedPortRanges, gc.HasLen, 1)
	c.Check(groupedPortRanges[port.WildcardEndpoint], gc.HasLen, 1)
	c.Check(groupedPortRanges[port.WildcardEndpoint][0], jc.DeepEquals, network.PortRange{Protocol: "tcp", FromPort: 1000, ToPort: 1500})
}

func (s *stateSuite) TestUpdateUnitPortsOpenOnInvalidEndpoint(c *gc.C) {
	st := NewState(s.TxnRunnerFactory())
	ctx := context.Background()

	err := st.RunAtomic(ctx, func(ctx domain.AtomicContext) error {
		return st.UpdateUnitPorts(ctx, s.unitUUID, network.GroupedPortRanges{
			"invalid": {{Protocol: "tcp", FromPort: 1000, ToPort: 1500}},
		}, network.GroupedPortRanges{})
	})
	c.Assert(err, jc.ErrorIs, porterrors.InvalidEndpoint)
}

func (s *stateSuite) TestUpdateUnitPortsClosePort(c *gc.C) {
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

	err = st.RunAtomic(ctx, func(ctx domain.AtomicContext) error {
		return st.UpdateUnitPorts(ctx, s.unitUUID, network.GroupedPortRanges{}, network.GroupedPortRanges{"ep0": {{Protocol: "tcp", FromPort: 80, ToPort: 80}}})
	})
	c.Assert(err, jc.ErrorIsNil)

	groupedPortRanges, err = st.GetUnitOpenedPorts(ctx, s.unitUUID)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(groupedPortRanges, gc.HasLen, 2)
	c.Check(groupedPortRanges["ep0"], gc.HasLen, 1)
	c.Check(groupedPortRanges["ep0"][0], jc.DeepEquals, network.PortRange{Protocol: "udp", FromPort: 1000, ToPort: 1500})

	c.Check(groupedPortRanges["ep1"], gc.HasLen, 1)
	c.Check(groupedPortRanges["ep1"][0], jc.DeepEquals, network.PortRange{Protocol: "tcp", FromPort: 8080, ToPort: 8080})
}

func (s *stateSuite) TestUpdateUnitPortsOpenPortRangeAdjacent(c *gc.C) {
	st := NewState(s.TxnRunnerFactory())
	ctx := context.Background()
	s.initialiseOpenPort(c, st)

	err := st.RunAtomic(ctx, func(ctx domain.AtomicContext) error {
		return st.UpdateUnitPorts(ctx, s.unitUUID, network.GroupedPortRanges{"ep0": {{Protocol: "udp", FromPort: 1501, ToPort: 2000}}}, network.GroupedPortRanges{})
	})
	c.Assert(err, jc.ErrorIsNil)

	groupedPortRanges, err := st.GetUnitOpenedPorts(ctx, s.unitUUID)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(groupedPortRanges, gc.HasLen, 2)
	c.Check(groupedPortRanges["ep0"], gc.HasLen, 3)
	c.Check(groupedPortRanges["ep0"][0], jc.DeepEquals, network.PortRange{Protocol: "tcp", FromPort: 80, ToPort: 80})
	c.Check(groupedPortRanges["ep0"][1], jc.DeepEquals, network.PortRange{Protocol: "udp", FromPort: 1000, ToPort: 1500})
	c.Check(groupedPortRanges["ep0"][2], jc.DeepEquals, network.PortRange{Protocol: "udp", FromPort: 1501, ToPort: 2000})

	c.Check(groupedPortRanges["ep1"], gc.HasLen, 1)
	c.Check(groupedPortRanges["ep1"][0], jc.DeepEquals, network.PortRange{Protocol: "tcp", FromPort: 8080, ToPort: 8080})
}

func (s *stateSuite) TestUpdateUnitPortsClosePortRange(c *gc.C) {
	st := NewState(s.TxnRunnerFactory())
	ctx := context.Background()
	s.initialiseOpenPort(c, st)

	err := st.RunAtomic(ctx, func(ctx domain.AtomicContext) error {
		return st.UpdateUnitPorts(ctx, s.unitUUID, network.GroupedPortRanges{}, network.GroupedPortRanges{"ep0": {{Protocol: "udp", FromPort: 1000, ToPort: 1500}}})
	})
	c.Assert(err, jc.ErrorIsNil)

	groupedPortRanges, err := st.GetUnitOpenedPorts(ctx, s.unitUUID)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(groupedPortRanges, gc.HasLen, 2)
	c.Check(groupedPortRanges["ep0"], gc.HasLen, 1)
	c.Check(groupedPortRanges["ep0"][0], jc.DeepEquals, network.PortRange{Protocol: "tcp", FromPort: 80, ToPort: 80})

	c.Check(groupedPortRanges["ep1"], gc.HasLen, 1)
	c.Check(groupedPortRanges["ep1"][0], jc.DeepEquals, network.PortRange{Protocol: "tcp", FromPort: 8080, ToPort: 8080})
}

func (s *stateSuite) TestUpdateUnitPortsClosePortEndpoint(c *gc.C) {
	st := NewState(s.TxnRunnerFactory())
	ctx := context.Background()
	s.initialiseOpenPort(c, st)

	err := st.RunAtomic(ctx, func(ctx domain.AtomicContext) error {
		return st.UpdateUnitPorts(ctx, s.unitUUID, network.GroupedPortRanges{}, network.GroupedPortRanges{
			"ep0": {
				{Protocol: "tcp", FromPort: 80, ToPort: 80},
				{Protocol: "udp", FromPort: 1000, ToPort: 1500},
			},
		})
	})
	c.Assert(err, jc.ErrorIsNil)

	groupedPortRanges, err := st.GetUnitOpenedPorts(ctx, s.unitUUID)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(groupedPortRanges, gc.HasLen, 1)

	c.Check(groupedPortRanges["ep1"], gc.HasLen, 1)
	c.Check(groupedPortRanges["ep1"][0], jc.DeepEquals, network.PortRange{Protocol: "tcp", FromPort: 8080, ToPort: 8080})
}

func (s *stateSuite) TestUpdateUnitPortsOpenCloseICMP(c *gc.C) {
	st := NewState(s.TxnRunnerFactory())
	ctx := context.Background()
	s.initialiseOpenPort(c, st)

	err := st.RunAtomic(ctx, func(ctx domain.AtomicContext) error {
		return st.UpdateUnitPorts(ctx, s.unitUUID, network.GroupedPortRanges{"ep0": {{Protocol: "icmp"}}}, network.GroupedPortRanges{})
	})
	c.Assert(err, jc.ErrorIsNil)

	groupedPortRanges, err := st.GetUnitOpenedPorts(ctx, s.unitUUID)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(groupedPortRanges, gc.HasLen, 2)
	c.Check(groupedPortRanges["ep0"], gc.HasLen, 3)
	c.Check(groupedPortRanges["ep0"][0], jc.DeepEquals, network.PortRange{Protocol: "icmp"})
	c.Check(groupedPortRanges["ep0"][1], jc.DeepEquals, network.PortRange{Protocol: "tcp", FromPort: 80, ToPort: 80})
	c.Check(groupedPortRanges["ep0"][2], jc.DeepEquals, network.PortRange{Protocol: "udp", FromPort: 1000, ToPort: 1500})

	c.Check(groupedPortRanges["ep1"], gc.HasLen, 1)
	c.Check(groupedPortRanges["ep1"][0], jc.DeepEquals, network.PortRange{Protocol: "tcp", FromPort: 8080, ToPort: 8080})

	err = st.RunAtomic(ctx, func(ctx domain.AtomicContext) error {
		return st.UpdateUnitPorts(ctx, s.unitUUID, network.GroupedPortRanges{}, network.GroupedPortRanges{"ep0": {{Protocol: "icmp"}}})
	})
	c.Check(err, jc.ErrorIsNil)

	groupedPortRanges, err = st.GetUnitOpenedPorts(ctx, s.unitUUID)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(groupedPortRanges, gc.HasLen, 2)
	c.Check(groupedPortRanges["ep0"], gc.HasLen, 2)
	c.Check(groupedPortRanges["ep0"][0], jc.DeepEquals, network.PortRange{Protocol: "tcp", FromPort: 80, ToPort: 80})
	c.Check(groupedPortRanges["ep0"][1], jc.DeepEquals, network.PortRange{Protocol: "udp", FromPort: 1000, ToPort: 1500})

	c.Check(groupedPortRanges["ep1"], gc.HasLen, 1)
	c.Check(groupedPortRanges["ep1"][0], jc.DeepEquals, network.PortRange{Protocol: "tcp", FromPort: 8080, ToPort: 8080})
}

func (s *stateSuite) TestUpdateUnitPortsOpenPortRangeMixedEndpoints(c *gc.C) {
	st := NewState(s.TxnRunnerFactory())
	ctx := context.Background()
	s.initialiseOpenPort(c, st)

	err := st.RunAtomic(ctx, func(ctx domain.AtomicContext) error {
		return st.UpdateUnitPorts(ctx, s.unitUUID, network.GroupedPortRanges{
			"ep0": {{Protocol: "udp", FromPort: 2500, ToPort: 3000}},
			"ep2": {{Protocol: "udp", FromPort: 2000, ToPort: 2100}},
		}, network.GroupedPortRanges{})
	})
	c.Assert(err, jc.ErrorIsNil)

	groupedPortRanges, err := st.GetUnitOpenedPorts(ctx, s.unitUUID)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(groupedPortRanges, gc.HasLen, 3)

	c.Check(groupedPortRanges["ep0"], gc.HasLen, 3)
	c.Check(groupedPortRanges["ep0"][0], jc.DeepEquals, network.PortRange{Protocol: "tcp", FromPort: 80, ToPort: 80})
	c.Check(groupedPortRanges["ep0"][1], jc.DeepEquals, network.PortRange{Protocol: "udp", FromPort: 1000, ToPort: 1500})
	c.Check(groupedPortRanges["ep0"][2], jc.DeepEquals, network.PortRange{Protocol: "udp", FromPort: 2500, ToPort: 3000})

	c.Check(groupedPortRanges["ep2"], gc.HasLen, 1)
	c.Check(groupedPortRanges["ep2"][0], jc.DeepEquals, network.PortRange{Protocol: "udp", FromPort: 2000, ToPort: 2100})

	c.Check(groupedPortRanges["ep1"], gc.HasLen, 1)
	c.Check(groupedPortRanges["ep1"][0], jc.DeepEquals, network.PortRange{Protocol: "tcp", FromPort: 8080, ToPort: 8080})
}

func (s *stateSuite) TestUpdateUnitPortsClosePortRangeMixedEndpoints(c *gc.C) {
	st := NewState(s.TxnRunnerFactory())
	ctx := context.Background()
	s.initialiseOpenPort(c, st)

	err := st.RunAtomic(ctx, func(ctx domain.AtomicContext) error {
		err := st.UpdateUnitPorts(ctx, s.unitUUID, network.GroupedPortRanges{
			"ep2": {
				{Protocol: "udp", FromPort: 2000, ToPort: 2500},
				{Protocol: "udp", FromPort: 3000, ToPort: 3000},
			},
		}, network.GroupedPortRanges{})
		if err != nil {
			return err
		}
		return st.UpdateUnitPorts(ctx, s.unitUUID, network.GroupedPortRanges{}, network.GroupedPortRanges{
			"ep0": {{Protocol: "udp", FromPort: 1000, ToPort: 1500}},
			"ep2": {{Protocol: "udp", FromPort: 2000, ToPort: 2500}},
		})
	})
	c.Assert(err, jc.ErrorIsNil)

	groupedPortRanges, err := st.GetUnitOpenedPorts(ctx, s.unitUUID)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(groupedPortRanges, gc.HasLen, 3)

	c.Check(groupedPortRanges["ep0"], gc.HasLen, 1)
	c.Check(groupedPortRanges["ep0"][0], jc.DeepEquals, network.PortRange{Protocol: "tcp", FromPort: 80, ToPort: 80})

	c.Check(groupedPortRanges["ep2"], gc.HasLen, 1)
	c.Check(groupedPortRanges["ep2"][0], jc.DeepEquals, network.PortRange{Protocol: "udp", FromPort: 3000, ToPort: 3000})

	c.Check(groupedPortRanges["ep1"], gc.HasLen, 1)
	c.Check(groupedPortRanges["ep1"][0], jc.DeepEquals, network.PortRange{Protocol: "tcp", FromPort: 8080, ToPort: 8080})
}

func (s *stateSuite) TestUpdateUnitPortRangesOpenAlreadyOpenAcrossUnits(c *gc.C) {
	st := NewState(s.TxnRunnerFactory())
	ctx := context.Background()
	s.initialiseOpenPort(c, st)
	unit1UUID, unit1Name := s.createUnit(c, netNodeUUIDs[0], appNames[0])

	err := st.RunAtomic(ctx, func(ctx domain.AtomicContext) error {
		return st.UpdateUnitPorts(ctx, s.unitUUID, network.GroupedPortRanges{"ep0": {{Protocol: "udp", FromPort: 1000, ToPort: 1500}}}, network.GroupedPortRanges{})
	})
	c.Assert(err, jc.ErrorIsNil)

	err = st.RunAtomic(ctx, func(ctx domain.AtomicContext) error {
		return st.UpdateUnitPorts(ctx, unit1UUID, network.GroupedPortRanges{"ep0": {{Protocol: "udp", FromPort: 1000, ToPort: 1500}}}, network.GroupedPortRanges{})
	})
	c.Assert(err, jc.ErrorIsNil)

	machineGroupedPortRanges, err := st.GetMachineOpenedPorts(ctx, machineUUIDs[0])
	c.Assert(err, jc.ErrorIsNil)
	c.Check(machineGroupedPortRanges, gc.HasLen, 2)

	unit0PortRanges, ok := machineGroupedPortRanges[s.unitName]
	c.Assert(ok, jc.IsTrue)
	c.Check(unit0PortRanges["ep0"], gc.HasLen, 2)
	c.Check(unit0PortRanges["ep0"][0], jc.DeepEquals, network.PortRange{Protocol: "tcp", FromPort: 80, ToPort: 80})
	c.Check(unit0PortRanges["ep0"][1], jc.DeepEquals, network.PortRange{Protocol: "udp", FromPort: 1000, ToPort: 1500})

	unit1PortRanges, ok := machineGroupedPortRanges[unit1Name]
	c.Assert(ok, jc.IsTrue)
	c.Check(unit1PortRanges, gc.HasLen, 1)

	c.Check(unit1PortRanges["ep0"], gc.HasLen, 1)
	c.Check(unit1PortRanges["ep0"][0], jc.DeepEquals, network.PortRange{Protocol: "udp", FromPort: 1000, ToPort: 1500})
}

func (s *stateSuite) TestUpdateUnitPortsMatchingRangeAcrossEndpoints(c *gc.C) {
	st := NewState(s.TxnRunnerFactory())
	ctx := context.Background()
	s.initialiseOpenPort(c, st)

	err := st.RunAtomic(ctx, func(ctx domain.AtomicContext) error {
		return st.UpdateUnitPorts(ctx, s.unitUUID, network.GroupedPortRanges{"ep2": {{Protocol: "udp", FromPort: 1000, ToPort: 1500}}}, network.GroupedPortRanges{})
	})
	c.Assert(err, jc.ErrorIsNil)

	groupedPortRanges, err := st.GetUnitOpenedPorts(ctx, s.unitUUID)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(groupedPortRanges, gc.HasLen, 3)

	c.Check(groupedPortRanges["ep0"], gc.HasLen, 2)
	c.Check(groupedPortRanges["ep0"][0], jc.DeepEquals, network.PortRange{Protocol: "tcp", FromPort: 80, ToPort: 80})
	c.Check(groupedPortRanges["ep0"][1], jc.DeepEquals, network.PortRange{Protocol: "udp", FromPort: 1000, ToPort: 1500})

	c.Check(groupedPortRanges["ep2"], gc.HasLen, 1)
	c.Check(groupedPortRanges["ep2"][0], jc.DeepEquals, network.PortRange{Protocol: "udp", FromPort: 1000, ToPort: 1500})

	c.Check(groupedPortRanges["ep1"], gc.HasLen, 1)
	c.Check(groupedPortRanges["ep1"][0], jc.DeepEquals, network.PortRange{Protocol: "tcp", FromPort: 8080, ToPort: 8080})
}

func (s *stateSuite) TestUpdateUnitPortRangesCloseAlreadyClosed(c *gc.C) {
	st := NewState(s.TxnRunnerFactory())
	ctx := context.Background()
	s.initialiseOpenPort(c, st)

	err := st.RunAtomic(ctx, func(ctx domain.AtomicContext) error {
		return st.UpdateUnitPorts(ctx, s.unitUUID, network.GroupedPortRanges{}, network.GroupedPortRanges{
			"ep0": {{Protocol: "tcp", FromPort: 7000, ToPort: 7000}},
		})
	})
	c.Assert(err, jc.ErrorIsNil)

	groupedPortRanges, err := st.GetUnitOpenedPorts(ctx, s.unitUUID)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(groupedPortRanges, gc.HasLen, 2)

	c.Check(groupedPortRanges["ep0"], gc.HasLen, 2)
	c.Check(groupedPortRanges["ep0"][0], jc.DeepEquals, network.PortRange{Protocol: "tcp", FromPort: 80, ToPort: 80})
	c.Check(groupedPortRanges["ep0"][1], jc.DeepEquals, network.PortRange{Protocol: "udp", FromPort: 1000, ToPort: 1500})

	c.Check(groupedPortRanges["ep1"], gc.HasLen, 1)
	c.Check(groupedPortRanges["ep1"][0], jc.DeepEquals, network.PortRange{Protocol: "tcp", FromPort: 8080, ToPort: 8080})
}

func (s *stateSuite) TestUpdateUnitPortRangeClosePortRangeWrongEndpoint(c *gc.C) {
	st := NewState(s.TxnRunnerFactory())
	ctx := context.Background()
	s.initialiseOpenPort(c, st)

	err := st.RunAtomic(ctx, func(ctx domain.AtomicContext) error {
		return st.UpdateUnitPorts(ctx, s.unitUUID, network.GroupedPortRanges{}, network.GroupedPortRanges{
			"ep1": {{Protocol: "tcp", FromPort: 80, ToPort: 80}},
		})
	})
	c.Check(err, jc.ErrorIsNil)

	groupedPortRanges, err := st.GetUnitOpenedPorts(ctx, s.unitUUID)
	c.Check(err, jc.ErrorIsNil)
	c.Check(groupedPortRanges, gc.HasLen, 2)

	c.Check(groupedPortRanges["ep0"], gc.HasLen, 2)
	c.Check(groupedPortRanges["ep0"][0], jc.DeepEquals, network.PortRange{Protocol: "tcp", FromPort: 80, ToPort: 80})
	c.Check(groupedPortRanges["ep0"][1], jc.DeepEquals, network.PortRange{Protocol: "udp", FromPort: 1000, ToPort: 1500})

	c.Check(groupedPortRanges["ep1"], gc.HasLen, 1)
	c.Check(groupedPortRanges["ep1"][0], jc.DeepEquals, network.PortRange{Protocol: "tcp", FromPort: 8080, ToPort: 8080})
}

func (s *stateSuite) TestUpdateUnitPortsOpenPortRangeAlreadyOpened(c *gc.C) {
	st := NewState(s.TxnRunnerFactory())
	ctx := context.Background()
	s.initialiseOpenPort(c, st)

	err := st.RunAtomic(ctx, func(ctx domain.AtomicContext) error {
		return st.UpdateUnitPorts(ctx, s.unitUUID, network.GroupedPortRanges{"ep0": {{Protocol: "tcp", FromPort: 80, ToPort: 80}}}, network.GroupedPortRanges{})
	})
	c.Assert(err, jc.ErrorIsNil)

	groupedPortRanges, err := st.GetUnitOpenedPorts(ctx, s.unitUUID)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(groupedPortRanges, gc.HasLen, 2)

	c.Check(groupedPortRanges["ep0"], gc.HasLen, 2)
	c.Check(groupedPortRanges["ep0"][0], jc.DeepEquals, network.PortRange{Protocol: "tcp", FromPort: 80, ToPort: 80})
	c.Check(groupedPortRanges["ep0"][1], jc.DeepEquals, network.PortRange{Protocol: "udp", FromPort: 1000, ToPort: 1500})

	c.Check(groupedPortRanges["ep1"], gc.HasLen, 1)
	c.Check(groupedPortRanges["ep1"][0], jc.DeepEquals, network.PortRange{Protocol: "tcp", FromPort: 8080, ToPort: 8080})
}

func (s *stateSuite) TestUpdateUnitPortsNilOpenPort(c *gc.C) {
	st := NewState(s.TxnRunnerFactory())
	ctx := context.Background()

	err := st.RunAtomic(ctx, func(ctx domain.AtomicContext) error {
		return st.UpdateUnitPorts(ctx, s.unitUUID, nil, nil)
	})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *stateSuite) TestGetEndpointsForPopulatedUnit(c *gc.C) {
	st := NewState(s.TxnRunnerFactory())
	ctx := context.Background()
	s.initialiseOpenPort(c, st)

	var endpoints []string
	err := st.RunAtomic(ctx, func(ctx domain.AtomicContext) error {
		var err error
		endpoints, err = st.GetEndpoints(ctx, s.unitUUID)
		return err
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Check(endpoints, jc.DeepEquals, []string{"ep0", "ep1", "ep2"})
}

func (s *stateSuite) TestGetEndpointsForUnpopulatedUnit(c *gc.C) {
	st := NewState(s.TxnRunnerFactory())
	ctx := context.Background()

	var endpoints []string
	err := st.RunAtomic(ctx, func(ctx domain.AtomicContext) error {
		var err error
		endpoints, err = st.GetEndpoints(ctx, s.unitUUID)
		return err
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Check(endpoints, jc.DeepEquals, []string{"ep0", "ep1", "ep2"})
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
