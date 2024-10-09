// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package stub

import (
	"context"
	"database/sql"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/domain/application"
	"github.com/juju/juju/domain/application/charm"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	applicationstate "github.com/juju/juju/domain/application/state"
	machineerrors "github.com/juju/juju/domain/machine/errors"
	machinestate "github.com/juju/juju/domain/machine/state"
	"github.com/juju/juju/domain/schema/testing"
	"github.com/juju/juju/internal/logger"
)

type stubSuite struct {
	testing.ModelSuite

	srv          *StubService
	appState     *applicationstate.ApplicationState
	machineState *machinestate.State
}

var _ = gc.Suite(&stubSuite{})

var addApplicationArg = application.AddApplicationArg{
	Charm: charm.Charm{
		Metadata: charm.Metadata{
			Name: "foo",
		},
	},
}

func (s *stubSuite) SetUpTest(c *gc.C) {
	s.ModelSuite.SetUpTest(c)
	s.srv = NewStubService(s.TxnRunnerFactory())
	s.appState = applicationstate.NewApplicationState(s.TxnRunnerFactory(), logger.GetLogger("juju.test.application"))
	s.machineState = machinestate.NewState(s.TxnRunnerFactory(), logger.GetLogger("juju.test.machine"))
}

func (s *stubSuite) TestAssignUnitsToMachines(c *gc.C) {
	_, err := s.appState.CreateApplication(context.Background(), "foo", addApplicationArg, application.UpsertUnitArg{
		UnitName: "foo/0",
	})
	c.Assert(err, jc.ErrorIsNil)

	err = s.machineState.CreateMachine(context.Background(), "0", "net-node-init-uuid", "machine-uuid")
	c.Assert(err, jc.ErrorIsNil)

	err = s.srv.AssignUnitsToMachines(context.Background(), map[string][]string{
		"0": {"foo/0"},
	})
	c.Assert(err, jc.ErrorIsNil)

	// Check that the unit have been assigned to the machine.
	err = s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		var unitNodeUUID string
		err := tx.QueryRowContext(ctx, "SELECT net_node_uuid FROM unit WHERE name = 'foo/0'").Scan(&unitNodeUUID)
		if err != nil {
			return err
		}

		var machineNodeUUID string
		err = tx.QueryRowContext(ctx, "SELECT net_node_uuid FROM machine WHERE name = '0'").Scan(&machineNodeUUID)
		if err != nil {
			return err
		}
		c.Check(unitNodeUUID, gc.Equals, machineNodeUUID)
		return nil
	})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *stubSuite) TestAssignUnitsToMachinesMachineNotFound(c *gc.C) {
	_, err := s.appState.CreateApplication(context.Background(), "foo", addApplicationArg, application.UpsertUnitArg{
		UnitName: "foo/0",
	})
	c.Assert(err, jc.ErrorIsNil)

	err = s.srv.AssignUnitsToMachines(context.Background(), map[string][]string{
		"0": {"foo/0"},
	})
	c.Assert(err, jc.ErrorIs, machineerrors.MachineNotFound)
}

func (s *stubSuite) TestAssignUnitsToMachinesUnitNotFound(c *gc.C) {
	_, err := s.appState.CreateApplication(context.Background(), "foo", addApplicationArg, application.UpsertUnitArg{
		UnitName: "foo/0",
	})
	c.Assert(err, jc.ErrorIsNil)

	err = s.machineState.CreateMachine(context.Background(), "0", "net-node-init-uuid", "machine-uuid")
	c.Assert(err, jc.ErrorIsNil)

	err = s.srv.AssignUnitsToMachines(context.Background(), map[string][]string{
		"0": {"foo/0"},
	})
	c.Assert(err, jc.ErrorIsNil)

	err = s.srv.AssignUnitsToMachines(context.Background(), map[string][]string{
		"0": {"foo/0", "foo/1"},
	})
	c.Assert(err, jc.ErrorIs, applicationerrors.UnitNotFound)
}

func (s *stubSuite) TestAssignUnitsToMachinesMultipleUnitsSameMachine(c *gc.C) {
	_, err := s.appState.CreateApplication(context.Background(), "foo", addApplicationArg, application.UpsertUnitArg{
		UnitName: "foo/0",
	}, application.UpsertUnitArg{
		UnitName: "foo/1",
	})
	c.Assert(err, jc.ErrorIsNil)

	err = s.machineState.CreateMachine(context.Background(), "0", "net-node-init-uuid", "machine-uuid")
	c.Assert(err, jc.ErrorIsNil)

	err = s.srv.AssignUnitsToMachines(context.Background(), map[string][]string{
		"0": {"foo/0", "foo/1"},
	})
	c.Assert(err, jc.ErrorIsNil)

	// Check that the units have been assigned to the machine.
	err = s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		var machineNodeUUID string
		err := tx.QueryRowContext(ctx, "SELECT net_node_uuid FROM machine WHERE name = '0'").Scan(&machineNodeUUID)
		if err != nil {
			return err
		}

		var unitNodeUUID string
		err = tx.QueryRowContext(ctx, "SELECT net_node_uuid FROM unit WHERE name = 'foo/0'").Scan(&unitNodeUUID)
		if err != nil {
			return err
		}
		c.Check(unitNodeUUID, gc.Equals, machineNodeUUID)

		err = tx.QueryRowContext(ctx, "SELECT net_node_uuid FROM unit WHERE name = 'foo/1'").Scan(&unitNodeUUID)
		if err != nil {
			return err
		}
		c.Check(unitNodeUUID, gc.Equals, machineNodeUUID)

		return nil
	})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *stubSuite) TestAssignUnitsToMachinesAssignUnitAndLaterAddMore(c *gc.C) {
	_, err := s.appState.CreateApplication(context.Background(), "foo", addApplicationArg, application.UpsertUnitArg{
		UnitName: "foo/0",
	})
	c.Assert(err, jc.ErrorIsNil)

	err = s.machineState.CreateMachine(context.Background(), "0", "net-node-init-uuid", "machine-uuid")
	c.Assert(err, jc.ErrorIsNil)

	err = s.srv.AssignUnitsToMachines(context.Background(), map[string][]string{
		"0": {"foo/0"},
	})
	c.Assert(err, jc.ErrorIsNil)

	err = s.appState.AddUnits(context.Background(), "foo", application.UpsertUnitArg{
		UnitName: "foo/1",
	})
	c.Assert(err, jc.ErrorIsNil)

	err = s.srv.AssignUnitsToMachines(context.Background(), map[string][]string{
		"0": {"foo/1"},
	})
	c.Assert(err, jc.ErrorIsNil)

	// Check that the units are assigned to the same machine.
	err = s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		var machineNodeUUID string
		err := tx.QueryRowContext(ctx, "SELECT net_node_uuid FROM machine WHERE name = '0'").Scan(&machineNodeUUID)
		if err != nil {
			return err
		}

		var unitNodeUUID string
		err = tx.QueryRowContext(ctx, "SELECT net_node_uuid FROM unit WHERE name = 'foo/0'").Scan(&unitNodeUUID)
		if err != nil {
			return err
		}
		c.Check(unitNodeUUID, gc.Equals, machineNodeUUID)

		err = tx.QueryRowContext(ctx, "SELECT net_node_uuid FROM unit WHERE name = 'foo/1'").Scan(&unitNodeUUID)
		if err != nil {
			return err
		}
		c.Check(unitNodeUUID, gc.Equals, machineNodeUUID)

		return nil
	})
	c.Assert(err, jc.ErrorIsNil)
}
