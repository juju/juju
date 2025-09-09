// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"database/sql"
	"strings"

	"github.com/juju/tc"

	"github.com/juju/juju/domain/life"
	schematesting "github.com/juju/juju/domain/schema/testing"
	"github.com/juju/juju/internal/errors"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	internaluuid "github.com/juju/juju/internal/uuid"
)

type baseSuite struct {
	schematesting.ModelSuite

	state *State
}

func (s *baseSuite) SetUpTest(c *tc.C) {
	s.ModelSuite.SetUpTest(c)

	s.state = NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))
}

// runQuery executes the provided SQL query string using the current state's database connection.
func (s *baseSuite) runQuery(c *tc.C, query string, args ...any) {
	err := s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, query, args...)
		if err != nil {
			return errors.Errorf("%w: query: %s (args: %s)", err, query, args)
		}
		return nil
	})
	c.Assert(err, tc.ErrorIsNil, tc.Commentf("(Arrange) failed to populate DB: %v",
		errors.ErrorStack(err)))
}

// insertOperation adds a new operation to the database.
func (s *baseSuite) insertOperation(c *tc.C, uuid string) {
	s.runQuery(c, `
INSERT INTO operation (uuid, operation_id, summary, enqueued_at, parallel, execution_group)
VALUES (?, 1, 'test-operation', datetime('now'), false, 'test-group')`, uuid)
}

// insertOperationTask adds a new operation task to the database.
func (s *baseSuite) insertOperationTask(c *tc.C, taskUUID, operationUUID string) {
	s.runQuery(c, `
INSERT INTO operation_task (uuid, operation_uuid, task_id, enqueued_at)
VALUES (?, ?, 1, datetime('now'))`, taskUUID, operationUUID)
}

// insertOperationTaskWithID adds a new operation task to the database with a specific task ID.
func (s *baseSuite) insertOperationTaskWithID(c *tc.C, taskUUID, operationUUID string, taskID string, statusID string) {
	s.runQuery(c, `
INSERT INTO operation_task (uuid, operation_uuid, task_id, enqueued_at)
VALUES (?, ?, ?, datetime('now'))`, taskUUID, operationUUID, taskID)
	s.runQuery(c, `
INSERT INTO operation_task_status (task_uuid, status_id)
VALUES (?, ?)`, taskUUID, statusID)
}

// insertUnit adds a new unit to the database with all required dependencies.
func (s *baseSuite) insertUnit(c *tc.C, unitUUID, unitName string) {
	appUUID := internaluuid.MustNewUUID().String()
	charmUUID := internaluuid.MustNewUUID().String()
	spaceUUID := internaluuid.MustNewUUID().String()
	netNodeUUID := internaluuid.MustNewUUID().String()

	// Extract application name from unit name (e.g., "test-app-1/0" -> "test-app-1")
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
VALUES (?, ?, ?, ?, ?)`, appUUID, appName, life.Alive, charmUUID, spaceUUID)

	// Insert unit
	s.runQuery(c, `
INSERT INTO unit (uuid, name, life_id, application_uuid, net_node_uuid, charm_uuid)
VALUES (?, ?, ?, ?, ?, ?)`, unitUUID, unitName, life.Alive, appUUID, netNodeUUID, charmUUID)
}

// insertMachine adds a new machine to the database with all required dependencies.
func (s *baseSuite) insertMachine(c *tc.C, machineUUID, machineName string) {
	netNodeUUID := internaluuid.MustNewUUID().String()

	// Insert net_node first
	s.runQuery(c, `
INSERT INTO net_node (uuid)
VALUES (?)`, netNodeUUID)

	// Insert machine
	s.runQuery(c, `
INSERT INTO machine (uuid, name, life_id, net_node_uuid)
VALUES (?, ?, ?, ?)`, machineUUID, machineName, life.Alive, netNodeUUID)
}

// insertOperationUnitTask links an operation task to a unit.
func (s *baseSuite) insertOperationUnitTask(c *tc.C, taskUUID, unitUUID string) {
	s.runQuery(c, `
INSERT INTO operation_unit_task (task_uuid, unit_uuid)
VALUES (?, ?)`, taskUUID, unitUUID)
}

// insertOperationMachineTask links an operation task to a machine.
func (s *baseSuite) insertOperationMachineTask(c *tc.C, taskUUID, machineUUID string) {
	s.runQuery(c, `
INSERT INTO operation_machine_task (task_uuid, machine_uuid)
VALUES (?, ?)`, taskUUID, machineUUID)
}

// assertTaskStatus verifies that a task has the expected status.
func (s *baseSuite) assertTaskStatus(c *tc.C, taskUUID string, expectedStatusID int) {
	err := s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		var statusID int
		err := tx.QueryRowContext(ctx, `
SELECT status_id FROM operation_task_status WHERE task_uuid = ?`, taskUUID).Scan(&statusID)
		if err != nil {
			return err
		}
		c.Check(statusID, tc.Equals, expectedStatusID)
		return nil
	})
	c.Assert(err, tc.ErrorIsNil)
}

// insertCharm adds a new charm to the database.
func (s *baseSuite) insertCharm(c *tc.C, charmUUID, name string) {
	s.runQuery(c, `
INSERT INTO charm (uuid, reference_name, source_id, revision)
VALUES (?, ?, 0, 1)`, charmUUID, name)
}

// insertCharmAction adds a new charm action to the database.
func (s *baseSuite) insertCharmAction(c *tc.C, charmUUID, key, description string) {
	s.runQuery(c, `
INSERT INTO charm_action (charm_uuid, "key", description, parallel, execution_group, params)
VALUES (?, ?, ?, false, NULL, NULL)`, charmUUID, key, description)
}

// insertOperationAction links an operation to a charm action.
func (s *baseSuite) insertOperationAction(c *tc.C, operationUUID, charmUUID, charmActionKey string) {
	s.runQuery(c, `
INSERT INTO operation_action (operation_uuid, charm_uuid, charm_action_key)
VALUES (?, ?, ?)`, operationUUID, charmUUID, charmActionKey)
}

// insertOperationLog adds a log entry for an operation.
func (s *baseSuite) insertOperationLog(c *tc.C, taskUUID, content string) {
	s.runQuery(c, `
INSERT INTO operation_task_log (task_uuid, content, created_at)
VALUES (?, ?, datetime('now'))`, taskUUID, content)
}

// insertOperationParameter adds a parameter for an operation.
func (s *baseSuite) insertOperationParameter(c *tc.C, operationUUID, key, value string) {
	s.runQuery(c, `
INSERT INTO operation_parameter (operation_uuid, "key", value)
VALUES (?, ?, ?)`, operationUUID, key, value)
}

// insertObjectStoreMetadata adds object store metadata to the database.
func (s *baseSuite) insertObjectStoreMetadata(c *tc.C, uuid, sha256, sha384 string, size int, path string) {
	s.runQuery(c, `
INSERT INTO object_store_metadata (uuid, sha_256, sha_384, size)
VALUES (?, ?, ?, ?)`, uuid, sha256, sha384, size)

	s.runQuery(c, `
INSERT INTO object_store_metadata_path (path, metadata_uuid)
VALUES (?, ?)`, path, uuid)
}

// insertOperationTaskOutput links a task to its output in object store.
func (s *baseSuite) insertOperationTaskOutput(c *tc.C, taskUUID, storeUUID string) {
	s.runQuery(c, `
INSERT INTO operation_task_output (task_uuid, store_uuid)
VALUES (?, ?)`, taskUUID, storeUUID)
}
