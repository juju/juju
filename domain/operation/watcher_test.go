// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package operation_test

import (
	"context"
	"database/sql"
	"strings"
	"testing"

	"github.com/juju/clock"
	"github.com/juju/tc"

	"github.com/juju/juju/core/changestream"
	coremachine "github.com/juju/juju/core/machine"
	coreunit "github.com/juju/juju/core/unit"
	"github.com/juju/juju/core/watcher/watchertest"
	"github.com/juju/juju/domain"
	"github.com/juju/juju/domain/operation/service"
	"github.com/juju/juju/domain/operation/state"
	changestreamtesting "github.com/juju/juju/internal/changestream/testing"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	internaluuid "github.com/juju/juju/internal/uuid"
)

type watcherSuite struct {
	changestreamtesting.ModelSuite

	svc *service.WatchableService
}

func TestWatcherSuite(t *testing.T) {
	tc.Run(t, &watcherSuite{})
}

func (s *watcherSuite) SetUpTest(c *tc.C) {
	s.ModelSuite.SetUpTest(c)

	// Setup the watcher factory for the "operation" namespace.
	factory := changestream.NewWatchableDBFactoryForNamespace(s.GetWatchableDB, "operation")
	s.svc = service.NewWatchableService(
		state.NewState(
			s.TxnRunnerFactory(),
			loggertesting.WrapCheckLog(c),
		),
		clock.WallClock,
		loggertesting.WrapCheckLog(c),
		nil, // object store not needed for these tests.
		domain.NewWatcherFactory(factory, loggertesting.WrapCheckLog(c)),
	)
}

// runQuery is a helper method to run SQL queries, copying the pattern from state tests.
func (s *watcherSuite) runQuery(c *tc.C, query string, args ...interface{}) {
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, query, args...)
		return err
	})
	c.Assert(err, tc.ErrorIsNil)
}

// addUnit creates a unit with all required dependencies, copying the pattern from state tests.
func (s *watcherSuite) addUnit(c *tc.C, unitName string) internaluuid.UUID {
	appUUID := internaluuid.MustNewUUID().String()
	charmUUID := internaluuid.MustNewUUID().String()
	spaceUUID := internaluuid.MustNewUUID().String()
	netNodeUUID := internaluuid.MustNewUUID().String()

	// Extract application name from unit name (e.g., "test-app/0" -> "test-app")
	appName := unitName
	if slashIndex := strings.Index(unitName, "/"); slashIndex != -1 {
		appName = unitName[:slashIndex]
	}

	// Insert net_node first
	s.runQuery(c, `
INSERT INTO net_node (uuid)
VALUES (?)`, netNodeUUID)

	// Insert space first (use unique name to avoid conflicts)
	spaceName := "test-space-" + spaceUUID[:8]
	s.runQuery(c, `
INSERT INTO space (uuid, name)
VALUES (?, ?)`, spaceUUID, spaceName)

	// Insert charm (use unique name to avoid conflicts)
	charmName := "test-charm-" + charmUUID[:8]
	s.runQuery(c, `
INSERT INTO charm (uuid, source_id, reference_name, revision, available)
VALUES (?, 1, ?, 1, true)`, charmUUID, charmName)

	// Insert application with extracted name from unit name
	s.runQuery(c, `
INSERT INTO application (uuid, name, life_id, charm_uuid, space_uuid)
VALUES (?, ?, ?, ?, ?)`, appUUID, appName, "0", charmUUID, spaceUUID)

	unitUUID := internaluuid.MustNewUUID()
	// Insert unit
	s.runQuery(c, `
INSERT INTO unit (uuid, name, life_id, application_uuid, net_node_uuid, charm_uuid)
VALUES (?, ?, ?, ?, ?, ?)`, unitUUID.String(), unitName, "0", appUUID, netNodeUUID, charmUUID)
	return unitUUID
}

// addMachine creates a machine with all required dependencies, copying the pattern from state tests.
func (s *watcherSuite) addMachine(c *tc.C, machineName string) internaluuid.UUID {
	netNodeUUID := internaluuid.MustNewUUID().String()

	// Insert net_node first
	s.runQuery(c, `
INSERT INTO net_node (uuid)
VALUES (?)`, netNodeUUID)

	machineUUID := internaluuid.MustNewUUID()
	// Insert machine
	s.runQuery(c, `
INSERT INTO machine (uuid, name, life_id, net_node_uuid)
VALUES (?, ?, ?, ?)`, machineUUID.String(), machineName, "0", netNodeUUID)

	return machineUUID
}

// addOperation creates an operation, copying the pattern from state tests.
func (s *watcherSuite) addOperation(c *tc.C) internaluuid.UUID {
	uuid := internaluuid.MustNewUUID()
	s.runQuery(c, `
INSERT INTO operation (uuid, operation_id, summary, enqueued_at, parallel, execution_group)
VALUES (?, 1, 'test-operation', datetime('now'), false, 'test-group')`, uuid.String())
	return uuid
}

// addOperationTaskWithID creates an operation task with specific ID and status, copying the pattern from state tests.
func (s *watcherSuite) addOperationTaskWithID(c *tc.C, operationUUID internaluuid.UUID, taskID string, statusID int) internaluuid.UUID {
	uuid := internaluuid.MustNewUUID()
	s.runQuery(c, `
INSERT INTO operation_task (uuid, operation_uuid, task_id, enqueued_at)
VALUES (?, ?, ?, datetime('now'))`, uuid.String(), operationUUID.String(), taskID)
	s.runQuery(c, `
INSERT INTO operation_task_status (task_uuid, status_id)
VALUES (?, ?)`, uuid.String(), statusID)
	return uuid
}

// addOperationUnitTask links an operation task to a unit, copying the pattern from state tests.
func (s *watcherSuite) addOperationUnitTask(c *tc.C, taskUUID, unitUUID internaluuid.UUID) {
	s.runQuery(c, `
INSERT INTO operation_unit_task (task_uuid, unit_uuid)
VALUES (?, ?)`, taskUUID.String(), unitUUID.String())
}

// addOperationMachineTask links an operation task to a machine, copying the pattern from state tests.
func (s *watcherSuite) addOperationMachineTask(c *tc.C, taskUUID, machineUUID internaluuid.UUID) {
	s.runQuery(c, `
INSERT INTO operation_machine_task (task_uuid, machine_uuid)
VALUES (?, ?)`, taskUUID.String(), machineUUID.String())
}

// setTaskStatus updates the status of an existing task.
func (s *watcherSuite) setTaskStatus(c *tc.C, taskUUID internaluuid.UUID, statusID int) {
	s.runQuery(c, `
UPDATE operation_task_status SET status_id=?, updated_at=datetime('now') WHERE task_uuid=?
`, statusID, taskUUID.String())
}

const (
	statusError     = 0
	statusRunning   = 1
	statusPending   = 2
	statusFailed    = 3
	statusCancelled = 4
	statusCompleted = 5
	statusAborting  = 6
	statusAborted   = 7
)

func (s *watcherSuite) TestWatchUnitTaskNotifications_PendingNotEmitted(c *tc.C) {
	unitName := "foo/0"
	unitUUID := s.addUnit(c, unitName)

	operationUUID := s.addOperation(c)
	task1UUID := s.addOperationTaskWithID(c, operationUUID, "task-0", statusPending)
	task2UUID := s.addOperationTaskWithID(c, operationUUID, "task-1", statusPending)
	s.addOperationUnitTask(c, task1UUID, unitUUID)
	s.addOperationUnitTask(c, task2UUID, unitUUID)

	unitNameParsed, err := coreunit.NewName(unitName)
	c.Assert(err, tc.ErrorIsNil)
	watcher, err := s.svc.WatchUnitTaskNotifications(c.Context(), unitNameParsed)
	c.Assert(err, tc.ErrorIsNil)
	harness := watchertest.NewHarness(s, watchertest.NewWatcherC(c, watcher))

	// Change one task to running.
	harness.AddTest(c, func(c *tc.C) {
		s.setTaskStatus(c, task1UUID, statusRunning)
	}, func(w watchertest.WatcherC[[]string]) {
		w.Check(watchertest.SliceAssert([]string{"task-0"}))
	})

	// Change the other to cancelled.
	harness.AddTest(c, func(c *tc.C) {
		s.setTaskStatus(c, task2UUID, statusCancelled)
	}, func(w watchertest.WatcherC[[]string]) {
		w.Check(watchertest.SliceAssert([]string{"task-1"}))
	})

	// Initial event: empty because pending tasks are not emitted for units.
	harness.Run(c, nil)
}

func (s *watcherSuite) TestWatchUnitTaskNotifications_NonPendingEmitted(c *tc.C) {
	unitName := "bar/0"
	unitUUID := s.addUnit(c, unitName)

	operationUUID := s.addOperation(c)
	task0UUID := s.addOperationTaskWithID(c, operationUUID, "task-0", statusRunning)
	task1UUID := s.addOperationTaskWithID(c, operationUUID, "task-1", statusRunning)
	s.addOperationUnitTask(c, task0UUID, unitUUID)
	s.addOperationUnitTask(c, task1UUID, unitUUID)

	unitNameParsed, err := coreunit.NewName(unitName)
	c.Assert(err, tc.ErrorIsNil)
	watcher, err := s.svc.WatchUnitTaskNotifications(c.Context(), unitNameParsed)
	c.Assert(err, tc.ErrorIsNil)
	harness := watchertest.NewHarness(s, watchertest.NewWatcherC(c, watcher))

	harness.AddTest(c, func(c *tc.C) {
	}, func(w watchertest.WatcherC[[]string]) {
		w.AssertChange()
	})

	// Complete one task.
	harness.AddTest(c, func(c *tc.C) {
		s.setTaskStatus(c, task0UUID, statusCompleted)
	}, func(w watchertest.WatcherC[[]string]) {
		w.Check(watchertest.SliceAssert([]string{"task-0"}))
	})

	// Initial event: both running tasks should be emitted.
	harness.Run(c, []string{"task-0", "task-1"})
}

func (s *watcherSuite) TestWatchUnitTaskNotifications_MixedPendingAndNonPending(c *tc.C) {
	unitName := "baz/0"
	unitUUID := s.addUnit(c, unitName)

	// Create both pending and non-pending tasks
	operationUUID := s.addOperation(c)
	task0UUID := s.addOperationTaskWithID(c, operationUUID, "task-0", statusPending)
	task1UUID := s.addOperationTaskWithID(c, operationUUID, "task-1", statusRunning)
	s.addOperationUnitTask(c, task0UUID, unitUUID)
	s.addOperationUnitTask(c, task1UUID, unitUUID)

	unitNameParsed, err := coreunit.NewName(unitName)
	c.Assert(err, tc.ErrorIsNil)
	watcher, err := s.svc.WatchUnitTaskNotifications(c.Context(), unitNameParsed)
	c.Assert(err, tc.ErrorIsNil)
	harness := watchertest.NewHarness(s, watchertest.NewWatcherC(c, watcher))

	harness.AddTest(c, func(c *tc.C) {
	}, func(w watchertest.WatcherC[[]string]) {
		w.AssertChange()
	})

	// Change pending task to running.
	harness.AddTest(c, func(c *tc.C) {
		s.setTaskStatus(c, task0UUID, statusRunning)
	}, func(w watchertest.WatcherC[[]string]) {
		w.Check(watchertest.SliceAssert([]string{"task-0"}))
	})

	// Initial event: only non-pending tasks should be emitted.
	harness.Run(c, []string{"task-1"})
}

func (s *watcherSuite) TestWatchMachineTaskNotifications_AllTasksEmitted(c *tc.C) {
	machineName := "0"
	machineUUID := s.addMachine(c, machineName)

	operationUUID := s.addOperation(c)
	task0UUID := s.addOperationTaskWithID(c, operationUUID, "task-0", statusPending)
	task1UUID := s.addOperationTaskWithID(c, operationUUID, "task-1", statusRunning)
	s.addOperationMachineTask(c, task0UUID, machineUUID)
	s.addOperationMachineTask(c, task1UUID, machineUUID)

	watcher, err := s.svc.WatchMachineTaskNotifications(c.Context(), coremachine.Name(machineName))
	c.Assert(err, tc.ErrorIsNil)
	harness := watchertest.NewHarness(s, watchertest.NewWatcherC(c, watcher))

	harness.AddTest(c, func(c *tc.C) {}, func(w watchertest.WatcherC[[]string]) {
		w.AssertChange()
	})

	// Complete the running task.
	harness.AddTest(c, func(c *tc.C) {
		s.setTaskStatus(c, task1UUID, statusCompleted)
	}, func(w watchertest.WatcherC[[]string]) {
		w.Check(watchertest.SliceAssert([]string{"task-1"}))
	})

	// Initial event: all tasks should be emitted (both pending and non-pending).
	harness.Run(c, []string{"task-0", "task-1"})
}

func (s *watcherSuite) TestWatchMachineTaskNotifications_PendingToRunning(c *tc.C) {
	machineName := "1"
	machineUUID := s.addMachine(c, machineName)

	operationUUID := s.addOperation(c)
	taskUUID := s.addOperationTaskWithID(c, operationUUID, "task-1", statusPending)
	s.addOperationMachineTask(c, taskUUID, machineUUID)

	watcher, err := s.svc.WatchMachineTaskNotifications(c.Context(), coremachine.Name(machineName))
	c.Assert(err, tc.ErrorIsNil)
	harness := watchertest.NewHarness(s, watchertest.NewWatcherC(c, watcher))

	harness.AddTest(c, func(c *tc.C) {
	}, func(w watchertest.WatcherC[[]string]) {
		w.Check(watchertest.SliceAssert([]string{"task-1"}))
	})

	// Change to running.
	harness.AddTest(c, func(c *tc.C) {
		s.setTaskStatus(c, taskUUID, statusRunning)
	}, func(w watchertest.WatcherC[[]string]) {
		w.Check(watchertest.SliceAssert([]string{"task-1"}))
	})

	// Complete the task.
	harness.AddTest(c, func(c *tc.C) {
		s.setTaskStatus(c, taskUUID, statusCompleted)
	}, func(w watchertest.WatcherC[[]string]) {
		w.Check(watchertest.SliceAssert([]string{"task-1"}))
	})

	// Initial event: pending task should be emitted for machines.
	harness.Run(c, []string{"task-1"})
}

func (s *watcherSuite) TestWatchUnitTaskNotifications_NotFound(c *tc.C) {
	unitNameParsed, err := coreunit.NewName("missing/0")
	c.Assert(err, tc.ErrorIsNil)
	_, err = s.svc.WatchUnitTaskNotifications(c.Context(), unitNameParsed)
	c.Assert(err, tc.ErrorMatches, `.*unit "missing/0" not found.*`)
}

func (s *watcherSuite) TestWatchMachineTaskNotifications_NotFound(c *tc.C) {
	_, err := s.svc.WatchMachineTaskNotifications(c.Context(), coremachine.Name("999"))
	c.Assert(err, tc.ErrorMatches, `.*machine "999" not found.*`)
}
