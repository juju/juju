// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"database/sql"
	"fmt"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	coreapplication "github.com/juju/juju/core/application"
	"github.com/juju/juju/core/network"
	coreunit "github.com/juju/juju/core/unit"
	"github.com/juju/juju/domain"
	"github.com/juju/juju/domain/application"
	"github.com/juju/juju/domain/application/charm"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	applicationstate "github.com/juju/juju/domain/application/state"
	machinestate "github.com/juju/juju/domain/machine/state"
	"github.com/juju/juju/domain/port"
	"github.com/juju/juju/internal/changestream/testing"
	"github.com/juju/juju/internal/errors"
	"github.com/juju/juju/internal/logger"
)

type baseSuite struct {
	testing.ModelSuite
	unitCount int
}

type stateSuite struct {
	baseSuite

	unitUUID coreunit.UUID
	unitName string

	appUUID coreapplication.ID
}

var _ = gc.Suite(&stateSuite{})

var (
	machineUUIDs = []string{"machine-0-uuid", "machine-1-uuid"}
	netNodeUUIDs = []string{"net-node-0-uuid", "net-node-1-uuid"}
	appNames     = []string{"app-0-name", "app-1-name"}
)

func (s *stateSuite) SetUpTest(c *gc.C) {
	s.ModelSuite.SetUpTest(c)

	machineSt := machinestate.NewState(s.TxnRunnerFactory(), logger.GetLogger("juju.test.machine"))
	err := machineSt.CreateMachine(context.Background(), "m", netNodeUUIDs[0], machineUUIDs[0])
	c.Assert(err, jc.ErrorIsNil)

	s.unitUUID, s.unitName, s.appUUID = s.createUnit(c, netNodeUUIDs[0], appNames[0])
}

// createUnit creates a new unit in state and returns its UUID. The unit is assigned
// to the net node with uuid `netNodeUUID`.
func (s *baseSuite) createUnit(c *gc.C, netNodeUUID, appName string) (uuid coreunit.UUID, name string, appid coreapplication.ID) {
	applicationSt := applicationstate.NewApplicationState(s.TxnRunnerFactory(), logger.GetLogger("juju.test.application"))
	_, err := applicationSt.CreateApplication(context.Background(), appName, application.AddApplicationArg{
		Charm: charm.Charm{
			Metadata: charm.Metadata{
				Name: appName,
			},
		},
	})
	c.Assert(err == nil || errors.Is(err, applicationerrors.ApplicationAlreadyExists), jc.IsTrue)

	unitName := fmt.Sprintf("%s/%d", appName, s.unitCount)
	err = applicationSt.AddUnits(context.Background(), appName, application.UpsertUnitArg{UnitName: unitName})
	c.Assert(err, jc.ErrorIsNil)
	s.unitCount++

	var (
		unitUUID coreunit.UUID
		appUUID  coreapplication.ID
	)
	err = s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		err := tx.QueryRowContext(ctx, "SELECT uuid FROM unit WHERE name = ?", unitName).Scan(&unitUUID)
		if err != nil {
			return err
		}

		err = tx.QueryRowContext(ctx, "SELECT uuid FROM application WHERE name = ?", appName).Scan(&appUUID)
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
	return unitUUID, unitName, appUUID
}

func (s *stateSuite) initialiseOpenPort(c *gc.C, st *State) {
	ctx := context.Background()
	err := st.RunAtomic(ctx, func(ctx domain.AtomicContext) error {
		return st.UpdateUnitPorts(ctx, s.unitUUID, network.GroupedPortRanges{
			"endpoint": {
				{Protocol: "tcp", FromPort: 80, ToPort: 80},
				{Protocol: "udp", FromPort: 1000, ToPort: 1500},
			},
			"misc": {
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

	c.Check(groupedPortRanges["endpoint"], gc.HasLen, 2)
	c.Check(groupedPortRanges["endpoint"][0], jc.DeepEquals, network.PortRange{Protocol: "tcp", FromPort: 80, ToPort: 80})
	c.Check(groupedPortRanges["endpoint"][1], jc.DeepEquals, network.PortRange{Protocol: "udp", FromPort: 1000, ToPort: 1500})

	c.Check(groupedPortRanges["misc"], gc.HasLen, 1)
	c.Check(groupedPortRanges["misc"][0], jc.DeepEquals, network.PortRange{Protocol: "tcp", FromPort: 8080, ToPort: 8080})
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

	unit0PortRanges, ok := machineGroupedPortRanges[s.unitUUID]
	c.Assert(ok, jc.IsTrue)
	c.Check(unit0PortRanges, gc.HasLen, 2)

	c.Check(unit0PortRanges["endpoint"], gc.HasLen, 2)
	c.Check(unit0PortRanges["endpoint"][0], jc.DeepEquals, network.PortRange{Protocol: "tcp", FromPort: 80, ToPort: 80})
	c.Check(unit0PortRanges["endpoint"][1], jc.DeepEquals, network.PortRange{Protocol: "udp", FromPort: 1000, ToPort: 1500})

	c.Check(unit0PortRanges["misc"], gc.HasLen, 1)
	c.Check(unit0PortRanges["misc"][0], jc.DeepEquals, network.PortRange{Protocol: "tcp", FromPort: 8080, ToPort: 8080})
}

func (s *stateSuite) TestGetMachineOpenedPortsAcrossTwoUnits(c *gc.C) {
	st := NewState(s.TxnRunnerFactory())
	ctx := context.Background()
	s.initialiseOpenPort(c, st)

	unit1UUID, _, _ := s.createUnit(c, netNodeUUIDs[0], appNames[0])
	err := st.RunAtomic(ctx, func(ctx domain.AtomicContext) error {
		return st.UpdateUnitPorts(ctx, unit1UUID, network.GroupedPortRanges{
			"endpoint": {
				{Protocol: "tcp", FromPort: 443, ToPort: 443},
				{Protocol: "udp", FromPort: 2000, ToPort: 2500},
			},
		}, network.GroupedPortRanges{})
	})
	c.Assert(err, jc.ErrorIsNil)

	machineGroupedPortRanges, err := st.GetMachineOpenedPorts(ctx, machineUUIDs[0])
	c.Assert(err, jc.ErrorIsNil)
	c.Check(machineGroupedPortRanges, gc.HasLen, 2)

	unit0PortRanges, ok := machineGroupedPortRanges[s.unitUUID]
	c.Assert(ok, jc.IsTrue)
	c.Check(unit0PortRanges, gc.HasLen, 2)

	c.Check(unit0PortRanges["endpoint"], gc.HasLen, 2)
	c.Check(unit0PortRanges["endpoint"][0], jc.DeepEquals, network.PortRange{Protocol: "tcp", FromPort: 80, ToPort: 80})
	c.Check(unit0PortRanges["endpoint"][1], jc.DeepEquals, network.PortRange{Protocol: "udp", FromPort: 1000, ToPort: 1500})

	c.Check(unit0PortRanges["misc"], gc.HasLen, 1)
	c.Check(unit0PortRanges["misc"][0], jc.DeepEquals, network.PortRange{Protocol: "tcp", FromPort: 8080, ToPort: 8080})

	unit1PortRanges, ok := machineGroupedPortRanges[unit1UUID]
	c.Assert(ok, jc.IsTrue)
	c.Check(unit1PortRanges, gc.HasLen, 1)

	c.Check(unit1PortRanges["endpoint"], gc.HasLen, 2)
	c.Check(unit1PortRanges["endpoint"][0], jc.DeepEquals, network.PortRange{Protocol: "tcp", FromPort: 443, ToPort: 443})
	c.Check(unit1PortRanges["endpoint"][1], jc.DeepEquals, network.PortRange{Protocol: "udp", FromPort: 2000, ToPort: 2500})
}

func (s *stateSuite) TestGetMachineOpenedPortsAcrossTwoUnitsDifferentMachines(c *gc.C) {
	st := NewState(s.TxnRunnerFactory())
	ctx := context.Background()
	s.initialiseOpenPort(c, st)

	machineSt := machinestate.NewState(s.TxnRunnerFactory(), logger.GetLogger("juju.test.machine"))
	err := machineSt.CreateMachine(context.Background(), "m1", netNodeUUIDs[1], machineUUIDs[1])
	c.Assert(err, jc.ErrorIsNil)

	unit1UUID, _, _ := s.createUnit(c, netNodeUUIDs[1], appNames[0])
	err = st.RunAtomic(ctx, func(ctx domain.AtomicContext) error {
		return st.UpdateUnitPorts(ctx, unit1UUID, network.GroupedPortRanges{
			"endpoint": {
				{Protocol: "tcp", FromPort: 443, ToPort: 443},
				{Protocol: "udp", FromPort: 2000, ToPort: 2500},
			},
		}, network.GroupedPortRanges{})
	})
	c.Assert(err, jc.ErrorIsNil)

	machineGroupedPortRanges, err := st.GetMachineOpenedPorts(ctx, machineUUIDs[0])
	c.Assert(err, jc.ErrorIsNil)
	c.Check(machineGroupedPortRanges, gc.HasLen, 1)

	unit0PortRanges, ok := machineGroupedPortRanges[s.unitUUID]
	c.Assert(ok, jc.IsTrue)
	c.Check(unit0PortRanges, gc.HasLen, 2)

	c.Check(unit0PortRanges["endpoint"], gc.HasLen, 2)
	c.Check(unit0PortRanges["endpoint"][0], jc.DeepEquals, network.PortRange{Protocol: "tcp", FromPort: 80, ToPort: 80})
	c.Check(unit0PortRanges["endpoint"][1], jc.DeepEquals, network.PortRange{Protocol: "udp", FromPort: 1000, ToPort: 1500})

	c.Check(unit0PortRanges["misc"], gc.HasLen, 1)
	c.Check(unit0PortRanges["misc"][0], jc.DeepEquals, network.PortRange{Protocol: "tcp", FromPort: 8080, ToPort: 8080})

	machineGroupedPortRanges, err = st.GetMachineOpenedPorts(ctx, machineUUIDs[1])
	c.Assert(err, jc.ErrorIsNil)
	c.Check(machineGroupedPortRanges, gc.HasLen, 1)

	unit1PortRanges, ok := machineGroupedPortRanges[unit1UUID]
	c.Assert(ok, jc.IsTrue)
	c.Check(unit1PortRanges, gc.HasLen, 1)

	c.Check(unit1PortRanges["endpoint"], gc.HasLen, 2)
	c.Check(unit1PortRanges["endpoint"][0], jc.DeepEquals, network.PortRange{Protocol: "tcp", FromPort: 443, ToPort: 443})
	c.Check(unit1PortRanges["endpoint"][1], jc.DeepEquals, network.PortRange{Protocol: "udp", FromPort: 2000, ToPort: 2500})
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
		{Endpoint: "endpoint", UnitUUID: s.unitUUID, PortRange: network.PortRange{Protocol: "tcp", FromPort: 80, ToPort: 80}},
		{Endpoint: "endpoint", UnitUUID: s.unitUUID, PortRange: network.PortRange{Protocol: "udp", FromPort: 1000, ToPort: 1500}},
		{Endpoint: "misc", UnitUUID: s.unitUUID, PortRange: network.PortRange{Protocol: "tcp", FromPort: 8080, ToPort: 8080}},
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

	unit1UUID, _, _ := s.createUnit(c, netNodeUUIDs[1], appNames[0])
	err := st.RunAtomic(ctx, func(ctx domain.AtomicContext) error {
		return st.UpdateUnitPorts(ctx, unit1UUID, network.GroupedPortRanges{
			"endpoint": {
				{Protocol: "tcp", FromPort: 443, ToPort: 443},
				{Protocol: "udp", FromPort: 2000, ToPort: 2500},
			},
		}, network.GroupedPortRanges{})
	})
	c.Assert(err, jc.ErrorIsNil)

	expect := port.UnitEndpointPortRanges{
		{Endpoint: "endpoint", UnitUUID: s.unitUUID, PortRange: network.PortRange{Protocol: "tcp", FromPort: 80, ToPort: 80}},
		{Endpoint: "endpoint", UnitUUID: s.unitUUID, PortRange: network.PortRange{Protocol: "udp", FromPort: 1000, ToPort: 1500}},
		{Endpoint: "misc", UnitUUID: s.unitUUID, PortRange: network.PortRange{Protocol: "tcp", FromPort: 8080, ToPort: 8080}},
		{Endpoint: "endpoint", UnitUUID: unit1UUID, PortRange: network.PortRange{Protocol: "tcp", FromPort: 443, ToPort: 443}},
		{Endpoint: "endpoint", UnitUUID: unit1UUID, PortRange: network.PortRange{Protocol: "udp", FromPort: 2000, ToPort: 2500}},
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

	unit1UUID, _, app1UUID := s.createUnit(c, netNodeUUIDs[1], "app-name-1")
	err := st.RunAtomic(ctx, func(ctx domain.AtomicContext) error {
		return st.UpdateUnitPorts(ctx, unit1UUID, network.GroupedPortRanges{
			"endpoint": {
				{Protocol: "tcp", FromPort: 443, ToPort: 443},
				{Protocol: "udp", FromPort: 2000, ToPort: 2500},
			},
		}, network.GroupedPortRanges{})
	})
	c.Assert(err, jc.ErrorIsNil)

	expect := port.UnitEndpointPortRanges{
		{Endpoint: "endpoint", UnitUUID: s.unitUUID, PortRange: network.PortRange{Protocol: "tcp", FromPort: 80, ToPort: 80}},
		{Endpoint: "endpoint", UnitUUID: s.unitUUID, PortRange: network.PortRange{Protocol: "udp", FromPort: 1000, ToPort: 1500}},
		{Endpoint: "misc", UnitUUID: s.unitUUID, PortRange: network.PortRange{Protocol: "tcp", FromPort: 8080, ToPort: 8080}},
	}
	port.SortUnitEndpointPortRanges(expect)

	unitEndpointPortRanges, err := st.GetApplicationOpenedPorts(ctx, s.appUUID)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(unitEndpointPortRanges, gc.HasLen, 3)
	c.Check(unitEndpointPortRanges, jc.DeepEquals, expect)

	expect = port.UnitEndpointPortRanges{
		{Endpoint: "endpoint", UnitUUID: unit1UUID, PortRange: network.PortRange{Protocol: "tcp", FromPort: 443, ToPort: 443}},
		{Endpoint: "endpoint", UnitUUID: unit1UUID, PortRange: network.PortRange{Protocol: "udp", FromPort: 2000, ToPort: 2500}},
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

	unit1UUID, _, _ := s.createUnit(c, netNodeUUIDs[0], appNames[0])
	err := st.RunAtomic(ctx, func(ctx domain.AtomicContext) error {
		return st.UpdateUnitPorts(ctx, unit1UUID, network.GroupedPortRanges{
			"endpoint": {
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

	unit1UUID, _, _ := s.createUnit(c, netNodeUUIDs[1], appNames[0])
	err := st.RunAtomic(ctx, func(ctx domain.AtomicContext) error {
		return st.UpdateUnitPorts(ctx, unit1UUID, network.GroupedPortRanges{
			"endpoint": {
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
		portRanges, err := st.GetEndpointOpenedPorts(ctx, s.unitUUID, "endpoint")
		c.Assert(err, jc.ErrorIsNil)
		c.Check(portRanges, gc.HasLen, 2)
		c.Check(portRanges[0], jc.DeepEquals, network.PortRange{Protocol: "tcp", FromPort: 80, ToPort: 80})
		c.Check(portRanges[1], jc.DeepEquals, network.PortRange{Protocol: "udp", FromPort: 1000, ToPort: 1500})

		portRanges, err = st.GetEndpointOpenedPorts(ctx, s.unitUUID, "misc")
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
		portRanges, err := st.GetEndpointOpenedPorts(ctx, s.unitUUID, "other-endpoint")
		c.Assert(err, jc.ErrorIsNil)
		c.Check(portRanges, gc.HasLen, 0)
		return nil
	})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *stateSuite) TestSetUnitPorts(c *gc.C) {
	st := NewState(s.TxnRunnerFactory())
	ctx := context.Background()

	err := st.SetUnitPorts(ctx, s.unitName, network.GroupedPortRanges{
		"endpoint1": {
			{Protocol: "tcp", FromPort: 1000, ToPort: 1500},
			{Protocol: "udp", FromPort: 300, ToPort: 799},
		},
		"endpoint2": {{Protocol: "udp", FromPort: 800, ToPort: 1200}},
	})
	c.Assert(err, jc.ErrorIsNil)

	groupedPortRanges, err := st.GetUnitOpenedPorts(ctx, s.unitUUID)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(groupedPortRanges, gc.HasLen, 2)
	c.Check(groupedPortRanges["endpoint1"], gc.HasLen, 2)
	c.Check(groupedPortRanges["endpoint1"][0], jc.DeepEquals, network.PortRange{Protocol: "tcp", FromPort: 1000, ToPort: 1500})
	c.Check(groupedPortRanges["endpoint1"][1], jc.DeepEquals, network.PortRange{Protocol: "udp", FromPort: 300, ToPort: 799})

	c.Check(groupedPortRanges["endpoint2"], gc.HasLen, 1)
	c.Check(groupedPortRanges["endpoint2"][0], jc.DeepEquals, network.PortRange{Protocol: "udp", FromPort: 800, ToPort: 1200})
}

func (s *stateSuite) TestSetUnitPortsUnitNotFound(c *gc.C) {
	st := NewState(s.TxnRunnerFactory())
	ctx := context.Background()

	err := st.SetUnitPorts(ctx, "badName", network.GroupedPortRanges{})
	c.Assert(err, jc.ErrorIs, applicationerrors.UnitNotFound)
}

func (s *stateSuite) TestUpdateUnitPortsOpenPort(c *gc.C) {
	st := NewState(s.TxnRunnerFactory())
	ctx := context.Background()
	s.initialiseOpenPort(c, st)

	err := st.RunAtomic(ctx, func(ctx domain.AtomicContext) error {
		return st.UpdateUnitPorts(ctx, s.unitUUID, network.GroupedPortRanges{"endpoint": {{Protocol: "tcp", FromPort: 1000, ToPort: 1500}}}, network.GroupedPortRanges{})
	})
	c.Assert(err, jc.ErrorIsNil)

	groupedPortRanges, err := st.GetUnitOpenedPorts(ctx, s.unitUUID)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(groupedPortRanges, gc.HasLen, 2)
	c.Check(groupedPortRanges["endpoint"], gc.HasLen, 3)
	c.Check(groupedPortRanges["endpoint"][0], jc.DeepEquals, network.PortRange{Protocol: "tcp", FromPort: 80, ToPort: 80})
	c.Check(groupedPortRanges["endpoint"][1], jc.DeepEquals, network.PortRange{Protocol: "tcp", FromPort: 1000, ToPort: 1500})
	c.Check(groupedPortRanges["endpoint"][2], jc.DeepEquals, network.PortRange{Protocol: "udp", FromPort: 1000, ToPort: 1500})

	c.Check(groupedPortRanges["misc"], gc.HasLen, 1)
	c.Check(groupedPortRanges["misc"][0], jc.DeepEquals, network.PortRange{Protocol: "tcp", FromPort: 8080, ToPort: 8080})
}

func (s *stateSuite) TestUpdateUnitPortsClosePort(c *gc.C) {
	st := NewState(s.TxnRunnerFactory())
	ctx := context.Background()
	s.initialiseOpenPort(c, st)

	groupedPortRanges, err := st.GetUnitOpenedPorts(ctx, s.unitUUID)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(groupedPortRanges, gc.HasLen, 2)
	c.Check(groupedPortRanges["endpoint"], gc.HasLen, 2)
	c.Check(groupedPortRanges["endpoint"][0], jc.DeepEquals, network.PortRange{Protocol: "tcp", FromPort: 80, ToPort: 80})
	c.Check(groupedPortRanges["endpoint"][1], jc.DeepEquals, network.PortRange{Protocol: "udp", FromPort: 1000, ToPort: 1500})

	c.Check(groupedPortRanges["misc"], gc.HasLen, 1)
	c.Check(groupedPortRanges["misc"][0], jc.DeepEquals, network.PortRange{Protocol: "tcp", FromPort: 8080, ToPort: 8080})

	err = st.RunAtomic(ctx, func(ctx domain.AtomicContext) error {
		return st.UpdateUnitPorts(ctx, s.unitUUID, network.GroupedPortRanges{}, network.GroupedPortRanges{"endpoint": {{Protocol: "tcp", FromPort: 80, ToPort: 80}}})
	})
	c.Assert(err, jc.ErrorIsNil)

	groupedPortRanges, err = st.GetUnitOpenedPorts(ctx, s.unitUUID)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(groupedPortRanges, gc.HasLen, 2)
	c.Check(groupedPortRanges["endpoint"], gc.HasLen, 1)
	c.Check(groupedPortRanges["endpoint"][0], jc.DeepEquals, network.PortRange{Protocol: "udp", FromPort: 1000, ToPort: 1500})

	c.Check(groupedPortRanges["misc"], gc.HasLen, 1)
	c.Check(groupedPortRanges["misc"][0], jc.DeepEquals, network.PortRange{Protocol: "tcp", FromPort: 8080, ToPort: 8080})
}

func (s *stateSuite) TestUpdateUnitPortsOpenPortRangeAdjacent(c *gc.C) {
	st := NewState(s.TxnRunnerFactory())
	ctx := context.Background()
	s.initialiseOpenPort(c, st)

	err := st.RunAtomic(ctx, func(ctx domain.AtomicContext) error {
		return st.UpdateUnitPorts(ctx, s.unitUUID, network.GroupedPortRanges{"endpoint": {{Protocol: "udp", FromPort: 1501, ToPort: 2000}}}, network.GroupedPortRanges{})
	})
	c.Assert(err, jc.ErrorIsNil)

	groupedPortRanges, err := st.GetUnitOpenedPorts(ctx, s.unitUUID)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(groupedPortRanges, gc.HasLen, 2)
	c.Check(groupedPortRanges["endpoint"], gc.HasLen, 3)
	c.Check(groupedPortRanges["endpoint"][0], jc.DeepEquals, network.PortRange{Protocol: "tcp", FromPort: 80, ToPort: 80})
	c.Check(groupedPortRanges["endpoint"][1], jc.DeepEquals, network.PortRange{Protocol: "udp", FromPort: 1000, ToPort: 1500})
	c.Check(groupedPortRanges["endpoint"][2], jc.DeepEquals, network.PortRange{Protocol: "udp", FromPort: 1501, ToPort: 2000})

	c.Check(groupedPortRanges["misc"], gc.HasLen, 1)
	c.Check(groupedPortRanges["misc"][0], jc.DeepEquals, network.PortRange{Protocol: "tcp", FromPort: 8080, ToPort: 8080})
}

func (s *stateSuite) TestUpdateUnitPortsClosePortRange(c *gc.C) {
	st := NewState(s.TxnRunnerFactory())
	ctx := context.Background()
	s.initialiseOpenPort(c, st)

	err := st.RunAtomic(ctx, func(ctx domain.AtomicContext) error {
		return st.UpdateUnitPorts(ctx, s.unitUUID, network.GroupedPortRanges{}, network.GroupedPortRanges{"endpoint": {{Protocol: "udp", FromPort: 1000, ToPort: 1500}}})
	})
	c.Assert(err, jc.ErrorIsNil)

	groupedPortRanges, err := st.GetUnitOpenedPorts(ctx, s.unitUUID)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(groupedPortRanges, gc.HasLen, 2)
	c.Check(groupedPortRanges["endpoint"], gc.HasLen, 1)
	c.Check(groupedPortRanges["endpoint"][0], jc.DeepEquals, network.PortRange{Protocol: "tcp", FromPort: 80, ToPort: 80})

	c.Check(groupedPortRanges["misc"], gc.HasLen, 1)
	c.Check(groupedPortRanges["misc"][0], jc.DeepEquals, network.PortRange{Protocol: "tcp", FromPort: 8080, ToPort: 8080})
}

func (s *stateSuite) TestUpdateUnitPortsClosePortEndpoint(c *gc.C) {
	st := NewState(s.TxnRunnerFactory())
	ctx := context.Background()
	s.initialiseOpenPort(c, st)

	err := st.RunAtomic(ctx, func(ctx domain.AtomicContext) error {
		return st.UpdateUnitPorts(ctx, s.unitUUID, network.GroupedPortRanges{}, network.GroupedPortRanges{
			"endpoint": {
				{Protocol: "tcp", FromPort: 80, ToPort: 80},
				{Protocol: "udp", FromPort: 1000, ToPort: 1500},
			},
		})
	})
	c.Assert(err, jc.ErrorIsNil)

	groupedPortRanges, err := st.GetUnitOpenedPorts(ctx, s.unitUUID)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(groupedPortRanges, gc.HasLen, 1)

	c.Check(groupedPortRanges["misc"], gc.HasLen, 1)
	c.Check(groupedPortRanges["misc"][0], jc.DeepEquals, network.PortRange{Protocol: "tcp", FromPort: 8080, ToPort: 8080})
}

func (s *stateSuite) TestUpdateUnitPortsOpenCloseICMP(c *gc.C) {
	st := NewState(s.TxnRunnerFactory())
	ctx := context.Background()
	s.initialiseOpenPort(c, st)

	err := st.RunAtomic(ctx, func(ctx domain.AtomicContext) error {
		return st.UpdateUnitPorts(ctx, s.unitUUID, network.GroupedPortRanges{"endpoint": {{Protocol: "icmp"}}}, network.GroupedPortRanges{})
	})
	c.Assert(err, jc.ErrorIsNil)

	groupedPortRanges, err := st.GetUnitOpenedPorts(ctx, s.unitUUID)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(groupedPortRanges, gc.HasLen, 2)
	c.Check(groupedPortRanges["endpoint"], gc.HasLen, 3)
	c.Check(groupedPortRanges["endpoint"][0], jc.DeepEquals, network.PortRange{Protocol: "icmp"})
	c.Check(groupedPortRanges["endpoint"][1], jc.DeepEquals, network.PortRange{Protocol: "tcp", FromPort: 80, ToPort: 80})
	c.Check(groupedPortRanges["endpoint"][2], jc.DeepEquals, network.PortRange{Protocol: "udp", FromPort: 1000, ToPort: 1500})

	c.Check(groupedPortRanges["misc"], gc.HasLen, 1)
	c.Check(groupedPortRanges["misc"][0], jc.DeepEquals, network.PortRange{Protocol: "tcp", FromPort: 8080, ToPort: 8080})

	err = st.RunAtomic(ctx, func(ctx domain.AtomicContext) error {
		return st.UpdateUnitPorts(ctx, s.unitUUID, network.GroupedPortRanges{}, network.GroupedPortRanges{"endpoint": {{Protocol: "icmp"}}})
	})
	c.Check(err, jc.ErrorIsNil)

	groupedPortRanges, err = st.GetUnitOpenedPorts(ctx, s.unitUUID)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(groupedPortRanges, gc.HasLen, 2)
	c.Check(groupedPortRanges["endpoint"], gc.HasLen, 2)
	c.Check(groupedPortRanges["endpoint"][0], jc.DeepEquals, network.PortRange{Protocol: "tcp", FromPort: 80, ToPort: 80})
	c.Check(groupedPortRanges["endpoint"][1], jc.DeepEquals, network.PortRange{Protocol: "udp", FromPort: 1000, ToPort: 1500})

	c.Check(groupedPortRanges["misc"], gc.HasLen, 1)
	c.Check(groupedPortRanges["misc"][0], jc.DeepEquals, network.PortRange{Protocol: "tcp", FromPort: 8080, ToPort: 8080})
}

func (s *stateSuite) TestUpdateUnitPortsOpenPortRangeMixedEndpoints(c *gc.C) {
	st := NewState(s.TxnRunnerFactory())
	ctx := context.Background()
	s.initialiseOpenPort(c, st)

	err := st.RunAtomic(ctx, func(ctx domain.AtomicContext) error {
		return st.UpdateUnitPorts(ctx, s.unitUUID, network.GroupedPortRanges{
			"endpoint":       {{Protocol: "udp", FromPort: 2500, ToPort: 3000}},
			"other-endpoint": {{Protocol: "udp", FromPort: 2000, ToPort: 2100}},
		}, network.GroupedPortRanges{})
	})
	c.Assert(err, jc.ErrorIsNil)

	groupedPortRanges, err := st.GetUnitOpenedPorts(ctx, s.unitUUID)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(groupedPortRanges, gc.HasLen, 3)

	c.Check(groupedPortRanges["endpoint"], gc.HasLen, 3)
	c.Check(groupedPortRanges["endpoint"][0], jc.DeepEquals, network.PortRange{Protocol: "tcp", FromPort: 80, ToPort: 80})
	c.Check(groupedPortRanges["endpoint"][1], jc.DeepEquals, network.PortRange{Protocol: "udp", FromPort: 1000, ToPort: 1500})
	c.Check(groupedPortRanges["endpoint"][2], jc.DeepEquals, network.PortRange{Protocol: "udp", FromPort: 2500, ToPort: 3000})

	c.Check(groupedPortRanges["other-endpoint"], gc.HasLen, 1)
	c.Check(groupedPortRanges["other-endpoint"][0], jc.DeepEquals, network.PortRange{Protocol: "udp", FromPort: 2000, ToPort: 2100})

	c.Check(groupedPortRanges["misc"], gc.HasLen, 1)
	c.Check(groupedPortRanges["misc"][0], jc.DeepEquals, network.PortRange{Protocol: "tcp", FromPort: 8080, ToPort: 8080})
}

func (s *stateSuite) TestUpdateUnitPortsClosePortRangeMixedEndpoints(c *gc.C) {
	st := NewState(s.TxnRunnerFactory())
	ctx := context.Background()
	s.initialiseOpenPort(c, st)

	err := st.RunAtomic(ctx, func(ctx domain.AtomicContext) error {
		err := st.UpdateUnitPorts(ctx, s.unitUUID, network.GroupedPortRanges{
			"other-endpoint": {
				{Protocol: "udp", FromPort: 2000, ToPort: 2500},
				{Protocol: "udp", FromPort: 3000, ToPort: 3000},
			},
		}, network.GroupedPortRanges{})
		if err != nil {
			return err
		}
		return st.UpdateUnitPorts(ctx, s.unitUUID, network.GroupedPortRanges{}, network.GroupedPortRanges{
			"endpoint":       {{Protocol: "udp", FromPort: 1000, ToPort: 1500}},
			"other-endpoint": {{Protocol: "udp", FromPort: 2000, ToPort: 2500}},
		})
	})
	c.Assert(err, jc.ErrorIsNil)

	groupedPortRanges, err := st.GetUnitOpenedPorts(ctx, s.unitUUID)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(groupedPortRanges, gc.HasLen, 3)

	c.Check(groupedPortRanges["endpoint"], gc.HasLen, 1)
	c.Check(groupedPortRanges["endpoint"][0], jc.DeepEquals, network.PortRange{Protocol: "tcp", FromPort: 80, ToPort: 80})

	c.Check(groupedPortRanges["other-endpoint"], gc.HasLen, 1)
	c.Check(groupedPortRanges["other-endpoint"][0], jc.DeepEquals, network.PortRange{Protocol: "udp", FromPort: 3000, ToPort: 3000})

	c.Check(groupedPortRanges["misc"], gc.HasLen, 1)
	c.Check(groupedPortRanges["misc"][0], jc.DeepEquals, network.PortRange{Protocol: "tcp", FromPort: 8080, ToPort: 8080})
}

func (s *stateSuite) TestUpdateUnitPortRangesOpenAlreadyOpenAcrossUnits(c *gc.C) {
	st := NewState(s.TxnRunnerFactory())
	ctx := context.Background()
	s.initialiseOpenPort(c, st)
	unit1UUID, _, _ := s.createUnit(c, netNodeUUIDs[0], appNames[0])

	err := st.RunAtomic(ctx, func(ctx domain.AtomicContext) error {
		return st.UpdateUnitPorts(ctx, s.unitUUID, network.GroupedPortRanges{"endpoint": {{Protocol: "udp", FromPort: 1000, ToPort: 1500}}}, network.GroupedPortRanges{})
	})
	c.Assert(err, jc.ErrorIsNil)

	err = st.RunAtomic(ctx, func(ctx domain.AtomicContext) error {
		return st.UpdateUnitPorts(ctx, unit1UUID, network.GroupedPortRanges{"endpoint": {{Protocol: "udp", FromPort: 1000, ToPort: 1500}}}, network.GroupedPortRanges{})
	})
	c.Assert(err, jc.ErrorIsNil)

	machineGroupedPortRanges, err := st.GetMachineOpenedPorts(ctx, machineUUIDs[0])
	c.Assert(err, jc.ErrorIsNil)
	c.Check(machineGroupedPortRanges, gc.HasLen, 2)

	unit0PortRanges, ok := machineGroupedPortRanges[s.unitUUID]
	c.Assert(ok, jc.IsTrue)
	c.Check(unit0PortRanges["endpoint"], gc.HasLen, 2)
	c.Check(unit0PortRanges["endpoint"][0], jc.DeepEquals, network.PortRange{Protocol: "tcp", FromPort: 80, ToPort: 80})
	c.Check(unit0PortRanges["endpoint"][1], jc.DeepEquals, network.PortRange{Protocol: "udp", FromPort: 1000, ToPort: 1500})

	unit1PortRanges, ok := machineGroupedPortRanges[unit1UUID]
	c.Assert(ok, jc.IsTrue)
	c.Check(unit1PortRanges, gc.HasLen, 1)

	c.Check(unit1PortRanges["endpoint"], gc.HasLen, 1)
	c.Check(unit1PortRanges["endpoint"][0], jc.DeepEquals, network.PortRange{Protocol: "udp", FromPort: 1000, ToPort: 1500})
}

func (s *stateSuite) TestUpdateUnitPortsMatchingRangeAcrossEndpoints(c *gc.C) {
	st := NewState(s.TxnRunnerFactory())
	ctx := context.Background()
	s.initialiseOpenPort(c, st)

	err := st.RunAtomic(ctx, func(ctx domain.AtomicContext) error {
		return st.UpdateUnitPorts(ctx, s.unitUUID, network.GroupedPortRanges{"other-endpoint": {{Protocol: "udp", FromPort: 1000, ToPort: 1500}}}, network.GroupedPortRanges{})
	})
	c.Assert(err, jc.ErrorIsNil)

	groupedPortRanges, err := st.GetUnitOpenedPorts(ctx, s.unitUUID)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(groupedPortRanges, gc.HasLen, 3)

	c.Check(groupedPortRanges["endpoint"], gc.HasLen, 2)
	c.Check(groupedPortRanges["endpoint"][0], jc.DeepEquals, network.PortRange{Protocol: "tcp", FromPort: 80, ToPort: 80})
	c.Check(groupedPortRanges["endpoint"][1], jc.DeepEquals, network.PortRange{Protocol: "udp", FromPort: 1000, ToPort: 1500})

	c.Check(groupedPortRanges["other-endpoint"], gc.HasLen, 1)
	c.Check(groupedPortRanges["other-endpoint"][0], jc.DeepEquals, network.PortRange{Protocol: "udp", FromPort: 1000, ToPort: 1500})

	c.Check(groupedPortRanges["misc"], gc.HasLen, 1)
	c.Check(groupedPortRanges["misc"][0], jc.DeepEquals, network.PortRange{Protocol: "tcp", FromPort: 8080, ToPort: 8080})
}

func (s *stateSuite) TestUpdateUnitPortRangesCloseAlreadyClosed(c *gc.C) {
	st := NewState(s.TxnRunnerFactory())
	ctx := context.Background()
	s.initialiseOpenPort(c, st)

	err := st.RunAtomic(ctx, func(ctx domain.AtomicContext) error {
		return st.UpdateUnitPorts(ctx, s.unitUUID, network.GroupedPortRanges{}, network.GroupedPortRanges{
			"endpoint": {{Protocol: "tcp", FromPort: 7000, ToPort: 7000}},
		})
	})
	c.Assert(err, jc.ErrorIsNil)

	groupedPortRanges, err := st.GetUnitOpenedPorts(ctx, s.unitUUID)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(groupedPortRanges, gc.HasLen, 2)

	c.Check(groupedPortRanges["endpoint"], gc.HasLen, 2)
	c.Check(groupedPortRanges["endpoint"][0], jc.DeepEquals, network.PortRange{Protocol: "tcp", FromPort: 80, ToPort: 80})
	c.Check(groupedPortRanges["endpoint"][1], jc.DeepEquals, network.PortRange{Protocol: "udp", FromPort: 1000, ToPort: 1500})

	c.Check(groupedPortRanges["misc"], gc.HasLen, 1)
	c.Check(groupedPortRanges["misc"][0], jc.DeepEquals, network.PortRange{Protocol: "tcp", FromPort: 8080, ToPort: 8080})
}

func (s *stateSuite) TestUpdateUnitPortRangeClosePortRangeWrongEndpoint(c *gc.C) {
	st := NewState(s.TxnRunnerFactory())
	ctx := context.Background()
	s.initialiseOpenPort(c, st)

	err := st.RunAtomic(ctx, func(ctx domain.AtomicContext) error {
		return st.UpdateUnitPorts(ctx, s.unitUUID, network.GroupedPortRanges{}, network.GroupedPortRanges{
			"misc": {{Protocol: "tcp", FromPort: 80, ToPort: 80}},
		})
	})
	c.Check(err, jc.ErrorIsNil)

	groupedPortRanges, err := st.GetUnitOpenedPorts(ctx, s.unitUUID)
	c.Check(err, jc.ErrorIsNil)
	c.Check(groupedPortRanges, gc.HasLen, 2)

	c.Check(groupedPortRanges["endpoint"], gc.HasLen, 2)
	c.Check(groupedPortRanges["endpoint"][0], jc.DeepEquals, network.PortRange{Protocol: "tcp", FromPort: 80, ToPort: 80})
	c.Check(groupedPortRanges["endpoint"][1], jc.DeepEquals, network.PortRange{Protocol: "udp", FromPort: 1000, ToPort: 1500})

	c.Check(groupedPortRanges["misc"], gc.HasLen, 1)
	c.Check(groupedPortRanges["misc"][0], jc.DeepEquals, network.PortRange{Protocol: "tcp", FromPort: 8080, ToPort: 8080})
}

func (s *stateSuite) TestUpdateUnitPortsOpenPortRangeAlreadyOpened(c *gc.C) {
	st := NewState(s.TxnRunnerFactory())
	ctx := context.Background()
	s.initialiseOpenPort(c, st)

	err := st.RunAtomic(ctx, func(ctx domain.AtomicContext) error {
		return st.UpdateUnitPorts(ctx, s.unitUUID, network.GroupedPortRanges{"endpoint": {{Protocol: "tcp", FromPort: 80, ToPort: 80}}}, network.GroupedPortRanges{})
	})
	c.Assert(err, jc.ErrorIsNil)

	groupedPortRanges, err := st.GetUnitOpenedPorts(ctx, s.unitUUID)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(groupedPortRanges, gc.HasLen, 2)

	c.Check(groupedPortRanges["endpoint"], gc.HasLen, 2)
	c.Check(groupedPortRanges["endpoint"][0], jc.DeepEquals, network.PortRange{Protocol: "tcp", FromPort: 80, ToPort: 80})
	c.Check(groupedPortRanges["endpoint"][1], jc.DeepEquals, network.PortRange{Protocol: "udp", FromPort: 1000, ToPort: 1500})

	c.Check(groupedPortRanges["misc"], gc.HasLen, 1)
	c.Check(groupedPortRanges["misc"][0], jc.DeepEquals, network.PortRange{Protocol: "tcp", FromPort: 8080, ToPort: 8080})
}

func (s *stateSuite) TestGetEndpoints(c *gc.C) {
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
	c.Check(endpoints, jc.DeepEquals, []string{"endpoint", "misc"})
}

func (s *stateSuite) TestGetEndpointsWithEmptyEndpoint(c *gc.C) {
	st := NewState(s.TxnRunnerFactory())
	ctx := context.Background()
	s.initialiseOpenPort(c, st)

	err := st.RunAtomic(ctx, func(ctx domain.AtomicContext) error {
		return st.UpdateUnitPorts(ctx, s.unitUUID,
			network.GroupedPortRanges{"other-endpoint": {}},
			network.GroupedPortRanges{"misc": {{Protocol: "tcp", FromPort: 8080, ToPort: 8080}}},
		)
	})
	c.Assert(err, jc.ErrorIsNil)

	err = st.RunAtomic(ctx, func(ctx domain.AtomicContext) error {
		endpoints, err := st.GetEndpoints(ctx, s.unitUUID)
		c.Check(endpoints, jc.DeepEquals, []string{"endpoint", "misc", "other-endpoint"})
		return err
	})
	c.Assert(err, jc.ErrorIsNil)
}
