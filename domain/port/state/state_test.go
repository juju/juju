// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/juju/collections/transform"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/network"
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
	"github.com/juju/juju/internal/uuid"
)

type stateSuite struct {
	testing.ModelSuite

	unitUUID  string
	unitCount int

	appUUID string

	epsUUIDMap map[string]string
}

var _ = gc.Suite(&stateSuite{})

const (
	mUUID       = "machine-uuid-0"
	netNodeUUID = "net-node-uuid-0"
	appName     = "app-name-0"
)

func (s *stateSuite) SetUpTest(c *gc.C) {
	s.ModelSuite.SetUpTest(c)

	machineSt := machinestate.NewState(s.TxnRunnerFactory(), logger.GetLogger("juju.test.machine"))
	err := machineSt.CreateMachine(context.Background(), "m", netNodeUUID, mUUID)
	c.Assert(err, jc.ErrorIsNil)

	s.unitUUID, s.appUUID = s.createUnit(c, netNodeUUID, appName)
	s.epsUUIDMap = map[string]string{}
}

// createUnit creates a new unit in state and returns its UUID. The unit is assigned
// to the net node with uuid `netNodeUUID`.
func (s *stateSuite) createUnit(c *gc.C, netNodeUUID, appName string) (string, string) {
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
		unitUUID string
		appUUID  string
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
	return unitUUID, appUUID
}

func (s *stateSuite) initialiseOpenPort(c *gc.C, st *State) {
	ctx := context.Background()
	err := st.RunAtomic(ctx, func(ctx domain.AtomicContext) error {
		var err error
		eps, err := st.AddEndpoints(ctx, s.unitUUID, []string{"endpoint", "misc", "other-endpoint"})
		if err != nil {
			return err
		}

		for _, ep := range eps {
			s.epsUUIDMap[ep.Endpoint] = ep.UUID
		}

		err = st.AddOpenedPorts(ctx, network.GroupedPortRanges{
			s.epsUUIDMap["endpoint"]: {
				{Protocol: "tcp", FromPort: 80, ToPort: 80},
				{Protocol: "udp", FromPort: 1000, ToPort: 1500},
			},
			s.epsUUIDMap["misc"]: {
				{Protocol: "tcp", FromPort: 8080, ToPort: 8080},
			},
		})
		if err != nil {
			return err
		}

		return nil
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

func (s *stateSuite) TestGetUnitOpenedPortsUUIDBlankDB(c *gc.C) {
	st := NewState(s.TxnRunnerFactory())
	ctx := context.Background()

	var groupedPortRanges map[string][]port.PortRangeUUID
	err := st.RunAtomic(ctx, func(ctx domain.AtomicContext) error {
		var err error
		groupedPortRanges, err = st.GetUnitOpenedPortsUUID(ctx, s.unitUUID)
		return err
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Check(groupedPortRanges, gc.HasLen, 0)

	err = st.RunAtomic(ctx, func(ctx domain.AtomicContext) error {
		var err error
		groupedPortRanges, err = st.GetUnitOpenedPortsUUID(ctx, "non-existent")
		return err
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Check(groupedPortRanges, gc.HasLen, 0)
}

func (s *stateSuite) TestGetUnitOpenedPortsUUID(c *gc.C) {
	st := NewState(s.TxnRunnerFactory())
	ctx := context.Background()
	s.initialiseOpenPort(c, st)

	var groupedPortRanges map[string][]port.PortRangeUUID
	err := st.RunAtomic(ctx, func(ctx domain.AtomicContext) error {
		var err error
		groupedPortRanges, err = st.GetUnitOpenedPortsUUID(ctx, s.unitUUID)
		return err
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Check(groupedPortRanges, gc.HasLen, 2)

	c.Check(groupedPortRanges["endpoint"], gc.HasLen, 2)

	c.Check(groupedPortRanges["endpoint"][0].PortRange, jc.DeepEquals, network.PortRange{Protocol: "tcp", FromPort: 80, ToPort: 80})
	c.Check(uuid.IsValidUUIDString(groupedPortRanges["endpoint"][0].UUID), jc.IsTrue)

	c.Check(groupedPortRanges["endpoint"][1].PortRange, jc.DeepEquals, network.PortRange{Protocol: "udp", FromPort: 1000, ToPort: 1500})
	c.Check(uuid.IsValidUUIDString(groupedPortRanges["endpoint"][1].UUID), jc.IsTrue)

	c.Check(groupedPortRanges["misc"], gc.HasLen, 1)
	c.Check(groupedPortRanges["misc"][0].PortRange, jc.DeepEquals, network.PortRange{Protocol: "tcp", FromPort: 8080, ToPort: 8080})
	c.Check(uuid.IsValidUUIDString(groupedPortRanges["misc"][0].UUID), jc.IsTrue)
}

func (s *stateSuite) TestGetMachineOpenedPortsBlankDB(c *gc.C) {
	st := NewState(s.TxnRunnerFactory())
	ctx := context.Background()

	machineGroupedPortRanges, err := st.GetMachineOpenedPorts(ctx, mUUID)
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

	machineGroupedPortRanges, err := st.GetMachineOpenedPorts(ctx, mUUID)
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

func (s *stateSuite) createUnit1(c *gc.C, st *State, netNodeUUID, appName string) (string, string) {
	ctx := context.Background()
	unit1UUID, app1UUID := s.createUnit(c, netNodeUUID, appName)

	err := st.RunAtomic(ctx, func(ctx domain.AtomicContext) error {
		eps, err := st.AddEndpoints(ctx, unit1UUID, []string{"endpoint"})
		if err != nil {
			return err
		}
		return st.AddOpenedPorts(ctx, network.GroupedPortRanges{
			eps[0].UUID: {
				{Protocol: "tcp", FromPort: 443, ToPort: 443},
				{Protocol: "udp", FromPort: 2000, ToPort: 2500},
			},
		})
	})
	c.Assert(err, jc.ErrorIsNil)

	return unit1UUID, app1UUID
}

func (s *stateSuite) TestGetMachineOpenedPortsAcrossTwoUnits(c *gc.C) {
	st := NewState(s.TxnRunnerFactory())
	ctx := context.Background()
	s.initialiseOpenPort(c, st)

	unit1UUID, _ := s.createUnit1(c, st, netNodeUUID, appName)

	machineGroupedPortRanges, err := st.GetMachineOpenedPorts(ctx, mUUID)
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
	err := machineSt.CreateMachine(context.Background(), "m2", "net-node-uuid-1", "machine-uuid-1")
	c.Assert(err, jc.ErrorIsNil)

	unit1UUID, _ := s.createUnit1(c, st, "net-node-uuid-1", appName)

	machineGroupedPortRanges, err := st.GetMachineOpenedPorts(ctx, mUUID)
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

	machineGroupedPortRanges, err = st.GetMachineOpenedPorts(ctx, "machine-uuid-1")
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

	unit1UUID, _ := s.createUnit1(c, st, "net-node-uuid-1", appName)

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

	unit1UUID, app1UUID := s.createUnit1(c, st, "net-node-uuid-1", "app-name-1")

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

	_, _ = s.createUnit1(c, st, netNodeUUID, appName)

	var opendPorts []network.PortRange
	err := st.RunAtomic(ctx, func(ctx domain.AtomicContext) error {
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

	_, _ = s.createUnit1(c, st, "net-node-uuid-1", appName)

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

func (s *stateSuite) TestGetColocatedOpenedPortsDedupes(c *gc.C) {
	st := NewState(s.TxnRunnerFactory())
	ctx := context.Background()
	s.initialiseOpenPort(c, st)

	unit1UUID, _ := s.createUnit1(c, st, netNodeUUID, appName)
	err := st.RunAtomic(ctx, func(ctx domain.AtomicContext) error {
		eps, err := st.AddEndpoints(ctx, unit1UUID, []string{"misc"})
		if err != nil {
			return err
		}
		return st.AddOpenedPorts(ctx, network.GroupedPortRanges{
			eps[0].UUID: {
				{Protocol: "tcp", FromPort: 8080, ToPort: 8080},
			},
		})
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

func (s *stateSuite) TestAddOpenedPorts(c *gc.C) {
	st := NewState(s.TxnRunnerFactory())
	ctx := context.Background()
	s.initialiseOpenPort(c, st)

	err := st.RunAtomic(ctx, func(ctx domain.AtomicContext) error {
		return st.AddOpenedPorts(ctx, network.GroupedPortRanges{
			s.epsUUIDMap["endpoint"]: {
				{Protocol: "tcp", FromPort: 1000, ToPort: 1500},
			},
		})
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

func (s *stateSuite) TestAddOpenedPortsICMP(c *gc.C) {
	st := NewState(s.TxnRunnerFactory())
	ctx := context.Background()
	s.initialiseOpenPort(c, st)

	err := st.RunAtomic(ctx, func(ctx domain.AtomicContext) error {
		return st.AddOpenedPorts(ctx, network.GroupedPortRanges{
			s.epsUUIDMap["endpoint"]: {
				{Protocol: "icmp"},
			},
		})
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
}

func (s *stateSuite) TestAddOpenedPortsAdjacentRange(c *gc.C) {
	st := NewState(s.TxnRunnerFactory())
	ctx := context.Background()
	s.initialiseOpenPort(c, st)

	err := st.RunAtomic(ctx, func(ctx domain.AtomicContext) error {
		return st.AddOpenedPorts(ctx, network.GroupedPortRanges{
			s.epsUUIDMap["endpoint"]: {
				{Protocol: "udp", FromPort: 1501, ToPort: 2000},
			},
		})
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

func (s *stateSuite) TestAddOpenedPortsAcrossEndpoints(c *gc.C) {
	st := NewState(s.TxnRunnerFactory())
	ctx := context.Background()
	s.initialiseOpenPort(c, st)

	err := st.RunAtomic(ctx, func(ctx domain.AtomicContext) error {
		return st.AddOpenedPorts(ctx, network.GroupedPortRanges{
			s.epsUUIDMap["endpoint"]:       {{Protocol: "udp", FromPort: 2500, ToPort: 3000}},
			s.epsUUIDMap["other-endpoint"]: {{Protocol: "udp", FromPort: 2000, ToPort: 2100}},
		})
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

func (s *stateSuite) TestRemoveOpenedPorts(c *gc.C) {
	st := NewState(s.TxnRunnerFactory())
	ctx := context.Background()
	s.initialiseOpenPort(c, st)

	err := st.RunAtomic(ctx, func(ctx domain.AtomicContext) error {
		ports, err := st.GetUnitOpenedPortsUUID(ctx, s.unitUUID)
		if err != nil {
			return err
		}
		var uuid string
		for _, portRange := range ports["endpoint"] {
			if portRange.PortRange == network.MustParsePortRange("80/tcp") {
				uuid = portRange.UUID
			}
		}
		return st.RemoveOpenedPorts(ctx, []string{uuid})
	})
	c.Assert(err, jc.ErrorIsNil)

	groupedPortRanges, err := st.GetUnitOpenedPorts(ctx, s.unitUUID)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(groupedPortRanges, gc.HasLen, 2)

	c.Check(groupedPortRanges["endpoint"], gc.HasLen, 1)
	c.Check(groupedPortRanges["endpoint"][0], jc.DeepEquals, network.PortRange{Protocol: "udp", FromPort: 1000, ToPort: 1500})

	c.Check(groupedPortRanges["misc"], gc.HasLen, 1)
	c.Check(groupedPortRanges["misc"][0], jc.DeepEquals, network.PortRange{Protocol: "tcp", FromPort: 8080, ToPort: 8080})
}

func (s *stateSuite) TestGetEndpoints(c *gc.C) {
	st := NewState(s.TxnRunnerFactory())
	ctx := context.Background()
	s.initialiseOpenPort(c, st)

	var endpoints []port.Endpoint
	err := st.RunAtomic(ctx, func(ctx domain.AtomicContext) error {
		var err error
		endpoints, err = st.GetEndpoints(ctx, s.unitUUID)
		return err
	})
	c.Assert(err, jc.ErrorIsNil)

	endpointNames := transform.Slice(endpoints, func(ep port.Endpoint) string { return ep.Endpoint })
	c.Check(endpointNames, jc.DeepEquals, []string{"endpoint", "misc", "other-endpoint"})

	for _, ep := range endpoints {
		c.Check(uuid.IsValidUUIDString(ep.UUID), jc.IsTrue)
	}
}

func (s *stateSuite) TestGetEndpointsAfterAddEndpoints(c *gc.C) {
	st := NewState(s.TxnRunnerFactory())
	ctx := context.Background()
	s.initialiseOpenPort(c, st)

	err := st.RunAtomic(ctx, func(ctx domain.AtomicContext) error {
		_, err := st.AddEndpoints(ctx, s.unitUUID, []string{"other-other-endpoint"})
		return err
	})
	c.Assert(err, jc.ErrorIsNil)

	var endpoints []port.Endpoint
	err = st.RunAtomic(ctx, func(ctx domain.AtomicContext) error {
		var err error
		endpoints, err = st.GetEndpoints(ctx, s.unitUUID)
		return err
	})
	c.Assert(err, jc.ErrorIsNil)

	endpointNames := transform.Slice(endpoints, func(ep port.Endpoint) string { return ep.Endpoint })
	c.Check(endpointNames, jc.DeepEquals, []string{"endpoint", "misc", "other-endpoint", "other-other-endpoint"})

	for _, ep := range endpoints {
		c.Check(uuid.IsValidUUIDString(ep.UUID), jc.IsTrue)
	}
}

func (s *stateSuite) TestAddEndpointsConflict(c *gc.C) {
	st := NewState(s.TxnRunnerFactory())
	ctx := context.Background()
	s.initialiseOpenPort(c, st)

	err := st.RunAtomic(ctx, func(ctx domain.AtomicContext) error {
		_, err := st.AddEndpoints(ctx, s.unitUUID, []string{"endpoint"})
		return err
	})
	c.Assert(err, jc.ErrorIs, ErrUnitEndpointConflict)
}
