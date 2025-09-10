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
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, query, args...)
		if err != nil {
			return errors.Errorf("%w: query: %s (args: %s)", err, query, args)
		}
		return nil
	})
	c.Assert(err, tc.ErrorIsNil, tc.Commentf("(Arrange) failed to populate DB: %v",
		errors.ErrorStack(err)))
}

// addOperation adds a new operation to the database.
func (s *baseSuite) addOperation(c *tc.C) internaluuid.UUID {
	uuid := internaluuid.MustNewUUID()
	s.runQuery(c, `
INSERT INTO operation (uuid, operation_id, summary, enqueued_at, parallel, execution_group)
VALUES (?, 1, 'test-operation', datetime('now'), false, 'test-group')`, uuid.String())
	return uuid
}

// addOperationTaskWithID adds a new operation task to the database with a specific task ID.
func (s *baseSuite) addOperationTaskWithID(c *tc.C, operationUUID internaluuid.UUID, taskID string, statusID string) internaluuid.UUID {
	uuid := internaluuid.MustNewUUID()
	s.runQuery(c, `
INSERT INTO operation_task (uuid, operation_uuid, task_id, enqueued_at)
VALUES (?, ?, ?, datetime('now'))`, uuid.String(), operationUUID.String(), taskID)
	s.runQuery(c, `
INSERT INTO operation_task_status (task_uuid, status_id)
VALUES (?, ?)`, uuid.String(), statusID)
	return uuid
}

// addUnit adds a new unit to the database with all required dependencies.
func (s *baseSuite) addUnit(c *tc.C) internaluuid.UUID {
	unitUUID := internaluuid.MustNewUUID()
	appUUID := internaluuid.MustNewUUID().String()
	charmUUID := internaluuid.MustNewUUID().String()
	spaceUUID := internaluuid.MustNewUUID().String()
	netNodeUUID := internaluuid.MustNewUUID().String()
	unitName := "test-app/0"

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
VALUES (?, ?, ?, ?, ?, ?)`, unitUUID.String(), unitName, life.Alive, appUUID, netNodeUUID, charmUUID)
	return unitUUID
}

// addMachine adds a new machine to the database with all required dependencies.
func (s *baseSuite) addMachine(c *tc.C) internaluuid.UUID {
	netNodeUUID := internaluuid.MustNewUUID().String()

	// Insert net_node first
	s.runQuery(c, `
INSERT INTO net_node (uuid)
VALUES (?)`, netNodeUUID)

	machineUUID := internaluuid.MustNewUUID()
	// Insert machine
	s.runQuery(c, `
INSERT INTO machine (uuid, name, life_id, net_node_uuid)
VALUES (?, ?, ?, ?)`, machineUUID.String(), "0", life.Alive, netNodeUUID)

	return machineUUID
}

// addOperationUnitTask links an operation task to a unit.
func (s *baseSuite) addOperationUnitTask(c *tc.C, taskUUID, unitUUID internaluuid.UUID) {
	s.runQuery(c, `
INSERT INTO operation_unit_task (task_uuid, unit_uuid)
VALUES (?, ?)`, taskUUID.String(), unitUUID.String())
}

// addOperationMachineTask links an operation task to a machine.
func (s *baseSuite) addOperationMachineTask(c *tc.C, taskUUID, machineUUID internaluuid.UUID) {
	s.runQuery(c, `
INSERT INTO operation_machine_task (task_uuid, machine_uuid)
VALUES (?, ?)`, taskUUID.String(), machineUUID.String())
}

// addCharm adds a new charm to the database.
func (s *baseSuite) addCharm(c *tc.C) internaluuid.UUID {
	charmUUID := internaluuid.MustNewUUID()
	s.runQuery(c, `
INSERT INTO charm (uuid, reference_name, source_id, revision)
VALUES (?, ?, 0, 1)`, charmUUID.String(), "test-charm")
	return charmUUID
}

// addCharmAction adds a new charm action to the database.
func (s *baseSuite) addCharmAction(c *tc.C, charmUUID internaluuid.UUID) {
	s.runQuery(c, `
INSERT INTO charm_action (charm_uuid, "key", description, parallel, execution_group, params)
VALUES (?, ?, ?, false, NULL, NULL)`, charmUUID.String(), "test-action", "Test action")
}

// addOperationAction links an operation to a charm action.
func (s *baseSuite) addOperationAction(c *tc.C, operationUUID, charmUUID internaluuid.UUID) {
	s.runQuery(c, `
INSERT INTO operation_action (operation_uuid, charm_uuid, charm_action_key)
VALUES (?, ?, ?)`, operationUUID.String(), charmUUID.String(), "test-action")
}

// addOperationLog adds a log entry for an operation.
func (s *baseSuite) addOperationLog(c *tc.C, taskUUID internaluuid.UUID, content string) {
	s.runQuery(c, `
INSERT INTO operation_task_log (task_uuid, content, created_at)
VALUES (?, ?, datetime('now'))`, taskUUID.String(), content)
}

// addOperationParameter adds a parameter for an operation.
func (s *baseSuite) addOperationParameter(c *tc.C, operationUUID internaluuid.UUID, key, value string) {
	s.runQuery(c, `
INSERT INTO operation_parameter (operation_uuid, "key", value)
VALUES (?, ?, ?)`, operationUUID.String(), key, value)
}

// addObjectStoreMetadata adds object store metadata to the database.
func (s *baseSuite) addObjectStoreMetadata(c *tc.C, sha256, sha384 string, size int, path string) internaluuid.UUID {
	uuid := internaluuid.MustNewUUID()
	s.runQuery(c, `
INSERT INTO object_store_metadata (uuid, sha_256, sha_384, size)
VALUES (?, ?, ?, ?)`, uuid.String(), sha256, sha384, size)

	s.runQuery(c, `
INSERT INTO object_store_metadata_path (path, metadata_uuid)
VALUES (?, ?)`, path, uuid.String())
	return uuid
}

// addOperationTaskOutput links a task to its output in object store.
func (s *baseSuite) addOperationTaskOutput(c *tc.C, taskUUID, storeUUID internaluuid.UUID) {
	s.runQuery(c, `
INSERT INTO operation_task_output (task_uuid, store_uuid)
VALUES (?, ?)`, taskUUID.String(), storeUUID.String())
}
