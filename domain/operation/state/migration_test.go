// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"testing"
	"time"

	"github.com/juju/tc"

	"github.com/juju/juju/core/machine"
	corestatus "github.com/juju/juju/core/status"
	coreunit "github.com/juju/juju/core/unit"
	"github.com/juju/juju/domain/operation/internal"
	internaluuid "github.com/juju/juju/internal/uuid"
)

type migrationSuite struct {
	baseSuite
}

func TestMigrationSuite(t *testing.T) {
	tc.Run(t, &migrationSuite{})
}

func (s *migrationSuite) SetUpTest(c *tc.C) {
	s.baseSuite.SetUpTest(c)
}

// TestImportOperationsNoArgsIsNoop ensures that passing an empty args slice
// results in no error and no DB changes.
func (s *migrationSuite) TestImportOperationsNoArgsIsNoop(c *tc.C) {
	// Arrange: ensure DB is empty of operations
	c.Check(s.getRowCount(c, "operation"), tc.Equals, 0)

	// Act
	err := s.state.InsertMigratingOperations(c.Context(), internal.ImportOperationsArgs{})

	// Assert
	c.Assert(err, tc.IsNil)
	c.Check(s.getRowCount(c, "operation"), tc.Equals, 0)
}

// TestImportOperationsWithSingleUnitTask exercises the happy path inserting an
// operation with a single unit task, including parameters, status and logs, and
// links an operation action via charm/application.
func (s *migrationSuite) TestImportOperationsWithSingleUnitTask(c *tc.C) {
	// Arrange
	charmUUID := s.addCharm(c)
	s.addCharmAction(c, charmUUID)               // ensures action key "test-action" exists
	s.addUnitWithName(c, charmUUID, "miniapp/0") // creates application "miniapp" and unit named "miniapp/0"

	now := time.Now().UTC()
	opUUID := internaluuid.MustNewUUID().String()
	taskUUID := internaluuid.MustNewUUID().String()
	enqOp := now.Add(-2 * time.Hour)
	startOp := now.Add(-90 * time.Minute)
	// Completed left zero to test nilZeroPtr
	enqTask := now.Add(-80 * time.Minute)
	startTask := now.Add(-70 * time.Minute)
	// Completed left zero to test nilZeroPtr
	logTime := now.Add(-1 * time.Minute)

	args := internal.ImportOperationsArgs{
		{
			ID:             "op-1",
			UUID:           opUUID,
			Summary:        "imported-op",
			Enqueued:       enqOp,
			Started:        startOp,
			Completed:      time.Time{},
			Status:         corestatus.Pending,
			IsParallel:     true,
			ExecutionGroup: "grp-1",
			Application:    "miniapp",
			ActionName:     "test-action",
			Parameters: map[string]any{
				"p1": "v1",
			},
			Tasks: []internal.ImportTaskArg{
				{
					ID:        "t-1",
					UUID:      taskUUID,
					UnitName:  coreunit.Name("miniapp/0"), // treated as a unit name
					Enqueued:  enqTask,
					Started:   startTask,
					Completed: time.Time{},
					Status:    corestatus.Running,
					Log:       []internal.TaskLogMessage{{Message: "hello", Timestamp: logTime}},
				},
			},
		},
	}

	// Act
	err := s.state.InsertMigratingOperations(c.Context(), args)

	// Assert
	c.Assert(err, tc.IsNil)

	// Verify operation row and timestamps/flags
	rows := s.queryRows(
		c, `
SELECT uuid, summary, enqueued_at, started_at, completed_at, parallel, execution_group 
FROM   operation 
WHERE  uuid = ?`, opUUID)
	c.Assert(rows, tc.HasLen, 1)
	row := rows[0]
	c.Check(row["uuid"], tc.Equals, opUUID)
	c.Check(row["summary"], tc.Equals, "imported-op")
	// started_at should be non-null, completed_at should be NULL
	c.Check(row["enqueued_at"], tc.Equals, enqOp)
	c.Check(row["started_at"], tc.Equals, startOp)
	c.Check(row["completed_at"], tc.IsNil)
	c.Check(row["parallel"], tc.Equals, true)
	c.Check(row["execution_group"], tc.Equals, "grp-1")

	// Verify task row
	taskRows := s.queryRows(c, `
SELECT uuid, operation_uuid, task_id, enqueued_at, started_at, completed_at 
FROM   operation_task 
WHERE  uuid = ?`, taskUUID)
	c.Assert(taskRows, tc.HasLen, 1)
	c.Check(taskRows[0]["operation_uuid"], tc.Equals, opUUID)
	c.Check(taskRows[0]["task_id"], tc.Equals, "t-1")
	c.Check(taskRows[0]["started_at"], tc.Equals, startTask)
	c.Check(taskRows[0]["enqueued_at"], tc.Equals, enqTask)
	c.Check(taskRows[0]["completed_at"], tc.IsNil)

	// Verify unit link exists and points to the "miniapp" unit
	unitRows := s.queryRows(c, `
SELECT name 
FROM   unit
JOIN   operation_unit_task AS out ON unit.uuid = out.unit_uuid
WHERE  task_uuid = ?`, taskUUID)
	c.Assert(unitRows, tc.HasLen, 1)
	c.Check(unitRows[0]["name"], tc.Equals, "miniapp/0")

	// Verify task status stored as "running"
	statusRows := s.queryRows(c, `
SELECT sv.status 
FROM   operation_task_status AS ts 
JOIN   operation_task_status_value AS sv ON ts.status_id = sv.id 
WHERE  ts.task_uuid = ?`, taskUUID)
	c.Assert(statusRows, tc.HasLen, 1)
	c.Check(statusRows[0]["status"], tc.Equals, "running")

	// Verify log entry stored
	logRows := s.queryRows(c, `
SELECT content, created_at
FROM   operation_task_log 
WHERE  task_uuid = ?`, taskUUID)
	c.Assert(logRows, tc.HasLen, 1)
	c.Check(logRows[0]["content"], tc.Equals, "hello")
	c.Check(logRows[0]["created_at"], tc.Equals, logTime)

	// Verify operation parameters saved from task parameters
	paramRows := s.queryRows(c, `
SELECT key, value 
FROM   operation_parameter 
WHERE  operation_uuid = ?`, opUUID)
	c.Assert(paramRows, tc.HasLen, 1)
	c.Check(paramRows[0]["key"], tc.Equals, "p1")
	c.Check(paramRows[0]["value"], tc.Equals, `"v1"`)

	// Verify operation action bound via application/charm
	actionRows := s.queryRows(c, `
SELECT charm_uuid, charm_action_key 
FROM   operation_action 
WHERE  operation_uuid = ?`, opUUID)
	c.Assert(actionRows, tc.HasLen, 1)
	c.Check(actionRows[0]["charm_action_key"], tc.Equals, "test-action")
	c.Check(actionRows[0]["charm_uuid"], tc.Equals, charmUUID)
}

// TestImportOperationsWithMultipleUnitTask tests the import of operations
// with multiple unit tasks into the system.
// Verifies proper assignment of tasks to units and checks for creation of
// correct database links.
func (s *migrationSuite) TestImportOperationsWithMultipleUnitTask(c *tc.C) {
	// Arrange
	charmUUID := s.addCharm(c)

	unitName1 := "miniapp/0"
	unitName2 := "miniapp/1"
	taskID1 := "t-u1"
	taskID2 := "t-u2"
	s.addCharmAction(c, charmUUID) // ensures action key "test-action" exists
	s.addUnitWithName(c, charmUUID, unitName1)
	s.addUnitWithName(c, charmUUID, unitName2)

	args := internal.ImportOperationsArgs{
		{
			ID:          "op-1",
			UUID:        internaluuid.MustNewUUID().String(),
			Enqueued:    time.Now(),
			Application: "miniapp",
			ActionName:  "test-action",
			Parameters:  map[string]any{"a": 1},
			Tasks: []internal.ImportTaskArg{
				{
					ID:       taskID1,
					UUID:     internaluuid.MustNewUUID().String(),
					UnitName: coreunit.Name(unitName1),
				},
				{
					ID:       taskID2,
					UUID:     internaluuid.MustNewUUID().String(),
					UnitName: coreunit.Name(unitName2),
				},
			},
		},
	}

	// Act
	err := s.state.InsertMigratingOperations(c.Context(), args)

	// Assert
	c.Assert(err, tc.IsNil)
	// Verify machine link exists and points to the right machine
	linkRows := s.queryRows(c, `
SELECT operation_task.task_id, unit.name 
FROM operation_unit_task
JOIN operation_task ON operation_task.uuid = operation_unit_task.task_uuid
JOIN unit ON unit.uuid = operation_unit_task.unit_uuid`)
	c.Check(linkRows, tc.SameContents, []map[string]any{
		{
			"task_id": taskID1,
			"name":    unitName1,
		},
		{
			"task_id": taskID2,
			"name":    unitName2,
		}})
}

// TestImportOperationsWithSingleMachineTask ensures machine receiver path works
// and output store link is created when provided.
func (s *migrationSuite) TestImportOperationsWithSingleMachineTask(c *tc.C) {
	// Arrange
	machineName := "0/lxd/1"
	machineUUID := s.addMachine(c, machineName)
	storeUUID := s.addFakeMetadataStore(c, 123)

	opUUID := internaluuid.MustNewUUID().String()
	taskUUID := internaluuid.MustNewUUID().String()

	args := internal.ImportOperationsArgs{
		{
			ID:         "op-m1",
			UUID:       opUUID,
			Enqueued:   time.Now().Add(-10 * time.Minute).UTC(),
			Started:    time.Time{}, // left zero
			Completed:  time.Time{},
			Parameters: map[string]any{"k": "v"},
			Tasks: []internal.ImportTaskArg{
				{
					ID:          "t-m1",
					UUID:        taskUUID,
					Status:      corestatus.Completed,
					StoreUUID:   storeUUID,
					MachineName: machine.Name(machineName),
				},
			},
		},
	}

	// Act
	err := s.state.InsertMigratingOperations(c.Context(), args)

	// Assert
	c.Assert(err, tc.IsNil)

	// Verify machine link exists and points to our machine
	linkRows := s.queryRows(c, `SELECT machine_uuid FROM operation_machine_task WHERE task_uuid = ?`, taskUUID)
	c.Assert(linkRows, tc.HasLen, 1)
	c.Check(linkRows[0]["machine_uuid"], tc.Equals, machineUUID)

	// Verify output link created
	outRows := s.queryRows(c, `SELECT store_uuid FROM operation_task_output WHERE task_uuid = ?`, taskUUID)
	c.Assert(outRows, tc.HasLen, 1)
	c.Check(outRows[0]["store_uuid"], tc.Equals, storeUUID)

	// Verify parameters at operation level were stored
	paramRows := s.queryRows(c, `SELECT key, value FROM operation_parameter WHERE operation_uuid = ?`, opUUID)
	c.Assert(paramRows, tc.HasLen, 1)
	c.Check(paramRows[0]["key"], tc.Equals, "k")
	c.Check(paramRows[0]["value"], tc.Equals, `"v"`)
}

// TestImportOperationsWithMultipleMachineTasks tests the import of operations
// with multiple machine tasks into the system.
func (s *migrationSuite) TestImportOperationsWithMultipleMachineTasks(c *tc.C) {
	// Arrange
	machineName1 := "0/lxd/1"
	machineName2 := "0"
	taskID1 := "t-m1"
	taskID2 := "t-m2"

	s.addMachine(c, machineName1)
	s.addMachine(c, machineName2)

	opUUID := internaluuid.MustNewUUID().String()
	taskUUID1 := internaluuid.MustNewUUID().String()
	taskUUID2 := internaluuid.MustNewUUID().String()

	args := internal.ImportOperationsArgs{
		{
			ID:       "op-m1",
			UUID:     opUUID,
			Enqueued: time.Now().Add(-10 * time.Minute).UTC(),
			Tasks: []internal.ImportTaskArg{
				{
					ID:          taskID1,
					UUID:        taskUUID1,
					MachineName: machine.Name(machineName1),
				},
				{
					ID:          taskID2,
					UUID:        taskUUID2,
					MachineName: machine.Name(machineName2),
				},
			},
		},
	}

	// Act
	err := s.state.InsertMigratingOperations(c.Context(), args)

	// Assert
	c.Assert(err, tc.IsNil)

	// Verify machine link exists and points to the right machine
	linkRows := s.queryRows(c, `
SELECT operation_task.task_id, machine.name 
FROM operation_machine_task
JOIN operation_task ON operation_task.uuid = operation_machine_task.task_uuid
JOIN machine ON machine.uuid = operation_machine_task.machine_uuid`)
	c.Check(linkRows, tc.SameContents, []map[string]any{
		{
			"task_id": taskID1,
			"name":    machineName1,
		},
		{
			"task_id": taskID2,
			"name":    machineName2,
		}})
}

// TestDeleteImportedOperations deletes all operations and returns referenced store paths.
func (s *migrationSuite) TestDeleteImportedOperations(c *tc.C) {
	// Arrange: create two operations, one with an output path
	op1 := s.addOperation(c)
	t1 := s.addOperationTask(c, op1)
	s.addOperationTaskOutputWithPath(c, t1, "/path/one")

	op2 := s.addOperation(c)
	s.addOperationTask(c, op2) // no path

	// Sanity: operations exist
	c.Assert(s.getRowCount(c, "operation") > 0, tc.IsTrue)

	// Act
	paths, err := s.state.DeleteImportedOperations(c.Context())

	// Assert
	c.Assert(err, tc.IsNil)
	c.Check(paths, tc.SameContents, []string{"/path/one"})
	c.Check(s.getRowCount(c, "operation"), tc.Equals, 0)
}

// TestDeleteImportedOperationsNoOps returns an empty list when there are no operations.
func (s *migrationSuite) TestDeleteImportedOperationsNoOps(c *tc.C) {
	// Act
	paths, err := s.state.DeleteImportedOperations(c.Context())
	// Assert
	c.Assert(err, tc.IsNil)
	c.Check(paths, tc.HasLen, 0)
}
