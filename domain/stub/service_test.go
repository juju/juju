// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package stub

import (
	"context"
	"database/sql"

	"github.com/juju/clock"
	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	modeltesting "github.com/juju/juju/core/model/testing"
	"github.com/juju/juju/core/unit"
	"github.com/juju/juju/domain/application"
	"github.com/juju/juju/domain/application/architecture"
	"github.com/juju/juju/domain/application/charm"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	applicationstate "github.com/juju/juju/domain/application/state"
	machineerrors "github.com/juju/juju/domain/machine/errors"
	machinestate "github.com/juju/juju/domain/machine/state"
	"github.com/juju/juju/domain/schema/testing"
	"github.com/juju/juju/internal/logger"
	coretesting "github.com/juju/juju/internal/testing"
)

type stubSuite struct {
	testing.ModelSuite

	srv          *StubService
	appState     *applicationstate.State
	machineState *machinestate.State
}

var _ = gc.Suite(&stubSuite{})

var addApplicationArg = application.AddApplicationArg{
	Charm: charm.Charm{
		Metadata: charm.Metadata{
			Name: "foo",
		},
		Manifest: charm.Manifest{
			Bases: []charm.Base{{
				Name:          "ubuntu",
				Channel:       charm.Channel{Risk: charm.RiskStable},
				Architectures: []string{"amd64"},
			}},
		},
		Source:        charm.LocalSource,
		Architecture:  architecture.AMD64,
		ReferenceName: "foo",
	},
}

func (s *stubSuite) TestAssignUnitsToMachines(c *gc.C) {
	defer s.setupMocks(c).Finish()

	_, err := s.appState.CreateApplication(context.Background(), "foo", addApplicationArg, []application.AddUnitArg{{UnitName: "foo/0"}})
	c.Assert(err, jc.ErrorIsNil)

	err = s.machineState.CreateMachine(context.Background(), "0", "net-node-init-uuid", "machine-uuid")
	c.Assert(err, jc.ErrorIsNil)

	err = s.srv.AssignUnitsToMachines(context.Background(), map[string][]unit.Name{
		"0": {"foo/0"},
	})
	c.Assert(err, jc.ErrorIsNil)

	// Check that the unit have been assigned to the machine.
	var unitNodeUUID string
	var machineNodeUUID string
	err = s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		err := tx.QueryRowContext(ctx, "SELECT net_node_uuid FROM unit WHERE name = 'foo/0'").Scan(&unitNodeUUID)
		if err != nil {
			return err
		}

		err = tx.QueryRowContext(ctx, "SELECT net_node_uuid FROM machine WHERE name = '0'").Scan(&machineNodeUUID)
		if err != nil {
			return err
		}

		return nil
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Check(unitNodeUUID, gc.Equals, machineNodeUUID)
}

func (s *stubSuite) TestAssignUnitsToMachinesMachineNotFound(c *gc.C) {
	defer s.setupMocks(c).Finish()

	_, err := s.appState.CreateApplication(context.Background(), "foo", addApplicationArg, []application.AddUnitArg{{UnitName: "foo/0"}})
	c.Assert(err, jc.ErrorIsNil)

	err = s.srv.AssignUnitsToMachines(context.Background(), map[string][]unit.Name{
		"0": {"foo/0"},
	})
	c.Assert(err, jc.ErrorIs, machineerrors.MachineNotFound)
}

func (s *stubSuite) TestAssignUnitsToMachinesUnitNotFound(c *gc.C) {
	defer s.setupMocks(c).Finish()

	_, err := s.appState.CreateApplication(context.Background(), "foo", addApplicationArg, []application.AddUnitArg{{UnitName: "foo/0"}})
	c.Assert(err, jc.ErrorIsNil)

	err = s.machineState.CreateMachine(context.Background(), "0", "net-node-init-uuid", "machine-uuid")
	c.Assert(err, jc.ErrorIsNil)

	err = s.srv.AssignUnitsToMachines(context.Background(), map[string][]unit.Name{
		"0": {"foo/0"},
	})
	c.Assert(err, jc.ErrorIsNil)

	err = s.srv.AssignUnitsToMachines(context.Background(), map[string][]unit.Name{
		"0": {"foo/0", "foo/1"},
	})
	c.Assert(err, jc.ErrorIs, applicationerrors.UnitNotFound)
}

func (s *stubSuite) TestAssignUnitsToMachinesMultipleUnitsSameMachine(c *gc.C) {
	defer s.setupMocks(c).Finish()

	_, err := s.appState.CreateApplication(context.Background(), "foo", addApplicationArg, []application.AddUnitArg{
		{UnitName: "foo/0"},
		{UnitName: "foo/1"},
	})
	c.Assert(err, jc.ErrorIsNil)

	err = s.machineState.CreateMachine(context.Background(), "0", "net-node-init-uuid", "machine-uuid")
	c.Assert(err, jc.ErrorIsNil)

	err = s.srv.AssignUnitsToMachines(context.Background(), map[string][]unit.Name{
		"0": {"foo/0", "foo/1"},
	})
	c.Assert(err, jc.ErrorIsNil)

	// Check that the units have been assigned to the machine.
	var machineNodeUUID string
	var unitNodeUUID0 string
	var unitNodeUUID1 string
	err = s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		err := tx.QueryRowContext(ctx, "SELECT net_node_uuid FROM machine WHERE name = '0'").Scan(&machineNodeUUID)
		if err != nil {
			return err
		}

		err = tx.QueryRowContext(ctx, "SELECT net_node_uuid FROM unit WHERE name = 'foo/0'").Scan(&unitNodeUUID0)
		if err != nil {
			return err
		}

		err = tx.QueryRowContext(ctx, "SELECT net_node_uuid FROM unit WHERE name = 'foo/1'").Scan(&unitNodeUUID1)
		if err != nil {
			return err
		}

		return nil
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Check(unitNodeUUID0, gc.Equals, machineNodeUUID)
	c.Check(unitNodeUUID1, gc.Equals, machineNodeUUID)
}

func (s *stubSuite) TestAssignUnitsToMachinesAssignUnitAndLaterAddMore(c *gc.C) {
	defer s.setupMocks(c).Finish()

	appID, err := s.appState.CreateApplication(context.Background(), "foo", addApplicationArg, []application.AddUnitArg{{UnitName: "foo/0"}})
	c.Assert(err, jc.ErrorIsNil)

	err = s.machineState.CreateMachine(context.Background(), "0", "net-node-init-uuid", "machine-uuid")
	c.Assert(err, jc.ErrorIsNil)

	err = s.srv.AssignUnitsToMachines(context.Background(), map[string][]unit.Name{
		"0": {"foo/0"},
	})
	c.Assert(err, jc.ErrorIsNil)

	err = s.appState.AddIAASUnits(context.Background(), c.MkDir(), appID, application.AddUnitArg{UnitName: "foo/1"})
	c.Assert(err, jc.ErrorIsNil)

	err = s.srv.AssignUnitsToMachines(context.Background(), map[string][]unit.Name{
		"0": {"foo/1"},
	})
	c.Assert(err, jc.ErrorIsNil)

	// Check that the units are assigned to the same machine.
	var machineNodeUUID string
	var unitNodeUUID0 string
	var unitNodeUUID1 string
	err = s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		err := tx.QueryRowContext(ctx, "SELECT net_node_uuid FROM machine WHERE name = '0'").Scan(&machineNodeUUID)
		if err != nil {
			return err
		}

		err = tx.QueryRowContext(ctx, "SELECT net_node_uuid FROM unit WHERE name = 'foo/0'").Scan(&unitNodeUUID0)
		if err != nil {
			return err
		}

		err = tx.QueryRowContext(ctx, "SELECT net_node_uuid FROM unit WHERE name = 'foo/1'").Scan(&unitNodeUUID1)
		if err != nil {
			return err
		}

		return nil
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Check(unitNodeUUID0, gc.Equals, machineNodeUUID)
	c.Check(unitNodeUUID1, gc.Equals, machineNodeUUID)
}

func (s *stubSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.srv = NewStubService(s.TxnRunnerFactory())
	s.appState = applicationstate.NewState(s.TxnRunnerFactory(), clock.WallClock, logger.GetLogger("juju.test.application"))
	s.machineState = machinestate.NewState(s.TxnRunnerFactory(), clock.WallClock, logger.GetLogger("juju.test.machine"))

	modelUUID := modeltesting.GenModelUUID(c)
	err := s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `
			INSERT INTO model (uuid, controller_uuid, name, type, cloud, cloud_type)
			VALUES (?, ?, "test", "iaas", "test-model", "ec2")
		`, modelUUID.String(), coretesting.ControllerTag.Id())
		return err
	})
	c.Assert(err, jc.ErrorIsNil)

	return ctrl
}
