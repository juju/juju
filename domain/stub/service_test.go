// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package stub

import (
	"context"
	"database/sql"

	jc "github.com/juju/testing/checkers"
	gomock "go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	coreapplication "github.com/juju/juju/core/application"
	"github.com/juju/juju/core/unit"
	"github.com/juju/juju/domain"
	"github.com/juju/juju/domain/application"
	"github.com/juju/juju/domain/application/charm"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	applicationstate "github.com/juju/juju/domain/application/state"
	machineerrors "github.com/juju/juju/domain/machine/errors"
	machinestate "github.com/juju/juju/domain/machine/state"
	"github.com/juju/juju/domain/schema/testing"
	"github.com/juju/juju/internal/errors"
	"github.com/juju/juju/internal/logger"
)

type stubSuite struct {
	testing.ModelSuite

	srv          *StubService
	appState     *applicationstate.ApplicationState
	machineState *machinestate.State

	storageRegistryGetter *MockModelStorageRegistryGetter
	storageRegistry       *MockProviderRegistry
}

var _ = gc.Suite(&stubSuite{})

var addApplicationArg = application.AddApplicationArg{
	Charm: charm.Charm{
		Metadata: charm.Metadata{
			Name: "foo",
		},
	},
}

func (s *stubSuite) TestAssignUnitsToMachines(c *gc.C) {
	defer s.setupMocks(c).Finish()

	err := s.appState.RunAtomic(context.Background(), func(ctx domain.AtomicContext) error {
		appID, err := s.appState.CreateApplication(ctx, "foo", addApplicationArg)
		if err != nil {
			return err
		}
		return s.appState.AddUnits(ctx, appID, application.AddUnitArg{UnitName: "foo/0"})
	})
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

	err := s.appState.RunAtomic(context.Background(), func(ctx domain.AtomicContext) error {
		appID, err := s.appState.CreateApplication(ctx, "foo", addApplicationArg)
		if err != nil {
			return err
		}
		return s.appState.AddUnits(ctx, appID, application.AddUnitArg{UnitName: "foo/0"})
	})
	c.Assert(err, jc.ErrorIsNil)

	err = s.srv.AssignUnitsToMachines(context.Background(), map[string][]unit.Name{
		"0": {"foo/0"},
	})
	c.Assert(err, jc.ErrorIs, machineerrors.MachineNotFound)
}

func (s *stubSuite) TestAssignUnitsToMachinesUnitNotFound(c *gc.C) {
	defer s.setupMocks(c).Finish()

	err := s.appState.RunAtomic(context.Background(), func(ctx domain.AtomicContext) error {
		appID, err := s.appState.CreateApplication(ctx, "foo", addApplicationArg)
		if err != nil {
			return err
		}
		return s.appState.AddUnits(ctx, appID, application.AddUnitArg{UnitName: "foo/0"})
	})
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

	err := s.appState.RunAtomic(context.Background(), func(ctx domain.AtomicContext) error {
		appID, err := s.appState.CreateApplication(ctx, "foo", addApplicationArg)
		if err != nil {
			return err
		}
		return s.appState.AddUnits(ctx, appID,
			application.AddUnitArg{UnitName: "foo/0"},
			application.AddUnitArg{UnitName: "foo/1"},
		)
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

	var appID coreapplication.ID
	err := s.appState.RunAtomic(context.Background(), func(ctx domain.AtomicContext) error {
		var err error
		appID, err = s.appState.CreateApplication(ctx, "foo", addApplicationArg)
		if err != nil {
			return err
		}
		return s.appState.AddUnits(ctx, appID, application.AddUnitArg{UnitName: "foo/0"})
	})
	c.Assert(err, jc.ErrorIsNil)

	err = s.machineState.CreateMachine(context.Background(), "0", "net-node-init-uuid", "machine-uuid")
	c.Assert(err, jc.ErrorIsNil)

	err = s.srv.AssignUnitsToMachines(context.Background(), map[string][]unit.Name{
		"0": {"foo/0"},
	})
	c.Assert(err, jc.ErrorIsNil)

	err = s.appState.RunAtomic(context.Background(), func(ctx domain.AtomicContext) error {
		return s.appState.AddUnits(ctx, appID, application.AddUnitArg{UnitName: "foo/1"})
	})
	c.Assert(err, jc.ErrorIsNil)
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

func (s *stubSuite) TestGetStorageRegistry(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.storageRegistryGetter.EXPECT().GetStorageRegistry(context.Background()).Return(s.storageRegistry, nil)

	reg, err := s.srv.GetStorageRegistry(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	c.Check(reg, gc.Equals, s.storageRegistry)
}

func (s *stubSuite) TestStorageRegistryError(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.storageRegistryGetter.EXPECT().GetStorageRegistry(context.Background()).Return(nil, errors.Errorf("boom"))

	_, err := s.srv.GetStorageRegistry(context.Background())
	c.Assert(err, gc.ErrorMatches, "getting storage registry: boom")
}

func (s *stubSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.storageRegistryGetter = NewMockModelStorageRegistryGetter(ctrl)
	s.storageRegistry = NewMockProviderRegistry(ctrl)

	s.srv = NewStubService(s.TxnRunnerFactory(), s.storageRegistryGetter)
	s.appState = applicationstate.NewApplicationState(s.TxnRunnerFactory(), logger.GetLogger("juju.test.application"))
	s.machineState = machinestate.NewState(s.TxnRunnerFactory(), logger.GetLogger("juju.test.machine"))

	return ctrl
}
