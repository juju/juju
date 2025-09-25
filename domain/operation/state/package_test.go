// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/canonical/sqlair"
	"github.com/juju/clock"
	"github.com/juju/tc"

	"github.com/juju/juju/core/network"
	coreunit "github.com/juju/juju/core/unit"
	"github.com/juju/juju/domain/life"
	schematesting "github.com/juju/juju/domain/schema/testing"
	"github.com/juju/juju/internal/errors"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	internaluuid "github.com/juju/juju/internal/uuid"
)

type baseSuite struct {
	schematesting.ModelSuite
	state  *State
	nextID func() string
}

// sequenceGenerator returns a function that generates unique string values in
// ascending order starting from "0".
func sequenceGenerator() func() string {
	id := 0
	return func() string {
		next := fmt.Sprint(id)
		id++
		return next
	}
}

func (s *baseSuite) SetUpTest(c *tc.C) {
	s.ModelSuite.SetUpTest(c)
	s.state = NewState(s.TxnRunnerFactory(), clock.WallClock, loggertesting.WrapCheckLog(c))
	s.nextID = sequenceGenerator()

	c.Cleanup(func() {
		s.state = nil
	})
}

// txn executes a transactional function within a database context,
// ensuring proper error handling and assertion.
func (s *baseSuite) txn(c *tc.C, fn func(ctx context.Context, tx *sqlair.TX) error) error {
	db, err := s.state.DB(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	return db.Txn(c.Context(), fn)
}

// query executes a given SQL query with optional arguments within a
// transactional context using the test database.
func (s *baseSuite) query(c *tc.C, query string, args ...any) {
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, query, args...)
		if err != nil {
			return errors.Errorf("%v: query: %s (args: %v)", err, query, args)
		}
		return nil
	})
	c.Assert(err, tc.ErrorIsNil, tc.Commentf("(Arrange) failed to populate DB: %v",
		errors.ErrorStack(err)))
}

// getRowCount returns the number of rows in a table.
func (s *baseSuite) getRowCount(c *tc.C, table string) int {
	var obtained int
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		query := fmt.Sprintf("SELECT COUNT(*) FROM %s", table)
		return tx.QueryRowContext(ctx, query).Scan(&obtained)
	})
	c.Assert(err, tc.IsNil, tc.Commentf("counting rows in table %q", table))
	return obtained
}

// getRowCountByField returns the number of rows in a table where field equals
// value.
func (s *baseSuite) getRowCountByField(c *tc.C, field, value, table string) int {
	var obtained int
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		query := fmt.Sprintf("SELECT COUNT(*) FROM %s WHERE %s = %q", table, field, value)
		return tx.QueryRowContext(ctx, query).Scan(&obtained)
	})
	c.Assert(err, tc.IsNil, tc.Commentf("counting rows in table %q for the field %q and value %q", table, field, value))
	return obtained
}

// selectDistinctValues retrieves distinct values for a given field from a table.
func (s *baseSuite) selectDistinctValues(c *tc.C, field, table string) []string {
	var obtained []string
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		query := fmt.Sprintf("SELECT DISTINCT %q FROM %q", field, table)
		rows, err := tx.QueryContext(ctx, query)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var val *string
			if err := rows.Scan(&val); err != nil {
				return err
			}
			if val == nil {
				obtained = append(obtained, "")
			} else {
				obtained = append(obtained, *val)
			}
		}
		return nil
	})
	c.Assert(err, tc.IsNil, tc.Commentf("fetching distinct %q from table %q", field, table))
	return obtained
}

// addCharm inserts a new charm record into the database and returns its UUID as a string.
func (s *baseSuite) addCharm(c *tc.C) string {
	charmUUID := internaluuid.MustNewUUID().String()
	s.query(c, `INSERT INTO charm (uuid, reference_name, create_time) VALUES (?, ?, ?)`,
		charmUUID, charmUUID, s.state.clock.Now())
	return charmUUID
}

// addCharmAction adds a new charm action to the database.
func (s *baseSuite) addCharmAction(c *tc.C, charmUUID string) {
	s.query(c, `
INSERT INTO charm_action (charm_uuid, "key", description, parallel, execution_group, params)
VALUES (?, ?, ?, false, NULL, NULL)`, charmUUID, "test-action", "Test action")
}

// addMachine inserts a new machine record and its associated net_node into the
// database, returning the machine UUID.
func (s *baseSuite) addMachine(c *tc.C, name string) string {
	netNodeUUID := internaluuid.MustNewUUID().String()
	machineUUID := internaluuid.MustNewUUID().String()
	s.query(c, `INSERT INTO net_node (uuid) VALUES (?)`, netNodeUUID)
	s.query(c, "INSERT INTO machine (uuid, net_node_uuid, name, life_id) VALUES (?, ?, ? ,?)",
		machineUUID, netNodeUUID, name, 0)
	return machineUUID
}

// addUnit inserts a new unit record into the database and returns the generated unit UUID.
func (s *baseSuite) addUnit(c *tc.C, charmUUID string) string {
	return s.addUnitWithName(c, charmUUID, "")
}

// addUnitWithName inserts a new unit record into the database and returns the generated unit UUID.
func (s *baseSuite) addUnitWithName(c *tc.C, charmUUID, name string) string {
	var unitName coreunit.Name
	if name != "" {
		unitName = coreunit.Name(name)
	} else {
		// Generate a unique application name to avoid conflicts
		// Use 'testapp' + ID to comply with Juju naming rules (no hyphens ending with numbers)
		uniqueAppName := fmt.Sprintf("testapp%s", s.nextID())
		unitName = coreunit.Name(fmt.Sprintf("%s/0", uniqueAppName))
	}

	appUUID := internaluuid.MustNewUUID().String()
	nodeUUID := internaluuid.MustNewUUID().String()
	unitUUID := internaluuid.MustNewUUID().String()
	s.query(c, `INSERT INTO net_node (uuid) VALUES (?)`, nodeUUID)
	s.query(c, `INSERT INTO application (uuid, name, life_id, charm_uuid, space_uuid) VALUES (?, ?, ?, ?, ?)`,
		appUUID, unitName.Application(), life.Alive, charmUUID, network.AlphaSpaceId)
	s.query(c, `INSERT INTO unit (uuid, name, life_id, application_uuid, charm_uuid, net_node_uuid) VALUES (?, ?, ?, ?, ?, ?)`,
		unitUUID, unitName.String(), life.Alive, appUUID, charmUUID, nodeUUID)
	return unitUUID
}

// addOperation inserts a minimal operation row, returning the new operation UUID.
func (s *baseSuite) addOperation(c *tc.C) string {
	return s.addOperationWithExecutionGroup(c, "")
}

// addOperationWithExecutionGroup inserts a minimal operation row, returning the new operation UUID.
func (s *baseSuite) addOperationWithExecutionGroup(c *tc.C, execGroup string) string {
	opUUID := internaluuid.MustNewUUID().String()
	opID := s.nextID()
	enqueued := s.state.clock.Now()
	s.query(c, `INSERT INTO operation (uuid, operation_id, summary, execution_group, enqueued_at) 
VALUES (?, ?, ?, ?,?)`, opUUID, opID,
		"", execGroup, enqueued)
	return opUUID
}

// addOperationTaskWithID adds a new operation task to the database with a specific task ID.
func (s *baseSuite) addOperationTaskWithID(c *tc.C, operationUUID string, taskID string, status string) string {
	taskUUID := internaluuid.MustNewUUID().String()
	s.query(c, `
INSERT INTO operation_task (uuid, operation_uuid, task_id, enqueued_at)
VALUES (?, ?, ?, datetime('now'))`, taskUUID, operationUUID, taskID)
	s.addOperationTaskStatus(c, taskUUID, status)
	return taskUUID
}

// addCompletedOperation inserts an operation with completed_at set to now - completedSince.
func (s *baseSuite) addCompletedOperation(c *tc.C, completedSince time.Duration) string {
	opUUID := internaluuid.MustNewUUID().String()
	opID := s.nextID()
	now := s.state.clock.Now()
	completedAt := now.Add(-completedSince)
	startedAt := completedAt.Add(-time.Second)
	enqueuedAt := startedAt.Add(-time.Second)
	s.query(c, `
INSERT INTO operation (uuid, operation_id, enqueued_at, started_at, completed_at) 
VALUES (?, ?, ?, ?, ?)`,
		opUUID, opID, enqueuedAt, startedAt, completedAt)
	return opUUID
}

// addOperationAction links an operation to a charm action; creates charm_action if necessary.
func (s *baseSuite) addOperationAction(c *tc.C, operationUUID, charmUUID, key string) {
	s.query(c, `INSERT INTO charm_action (charm_uuid, "key") VALUES (?, ?) ON CONFLICT DO NOTHING`, charmUUID, key)
	// Insert operation_action
	s.query(c, `INSERT INTO operation_action (operation_uuid, charm_uuid, charm_action_key) VALUES (?, ?, ?)`, operationUUID, charmUUID, key)
}

// addOperationTask inserts a minimal operation_task for the given operation.
func (s *baseSuite) addOperationTask(c *tc.C, operationUUID string) string {
	taskUUID := internaluuid.MustNewUUID().String()
	taskID := s.nextID()
	enqueued := s.state.clock.Now()
	s.query(c, `INSERT INTO operation_task (uuid, operation_uuid, task_id, enqueued_at) VALUES (?, ?, ?, ?)`, taskUUID, operationUUID, taskID, enqueued)
	return taskUUID
}

// addOperationUnitTask links a task to a unit
func (s *baseSuite) addOperationUnitTask(c *tc.C, taskUUID, unitUUID string) {
	s.query(c, `INSERT INTO operation_unit_task (task_uuid, unit_uuid) VALUES (?, ?)`, taskUUID, unitUUID)
}

// addOperationMachineTask links a task to a machine
func (s *baseSuite) addOperationMachineTask(c *tc.C, taskUUID, machineUUID string) {
	s.query(c, `INSERT INTO operation_machine_task (task_uuid, machine_uuid) VALUES (?, ?)`, taskUUID, machineUUID)
}

func (s *baseSuite) addFakeMetadataStore(c *tc.C, size int) string {
	storeUUID := internaluuid.MustNewUUID().String()
	s.query(c, `
INSERT INTO object_store_metadata (uuid, sha_256, sha_384, size)
VALUES (?, ?, ?, ?)`, storeUUID, storeUUID, storeUUID, size)
	return storeUUID
}

// addOperationTaskOutputWithData adds object store metadata to the database with a path
func (s *baseSuite) addOperationTaskOutputWithData(c *tc.C, taskUUID, sha256, sha384 string, size int,
	path string) string {
	storeUUID := internaluuid.MustNewUUID().String()
	s.query(c, `
INSERT INTO object_store_metadata (uuid, sha_256, sha_384, size)
VALUES (?, ?, ?, ?)`, storeUUID, sha256, sha384, size)
	s.addMetadataStorePath(c, storeUUID, path)
	s.query(c, `INSERT INTO operation_task_output (task_uuid, store_uuid) VALUES (?, ?)`, taskUUID, storeUUID)
	return storeUUID
}

// addOperationTaskOutput links a task to an object store metadata, return the
// store metadata uuid
func (s *baseSuite) addOperationTaskOutputWithPath(c *tc.C, taskUUID string, path string) string {
	storeUUID := s.addFakeMetadataStore(c, 42)
	s.query(c, `INSERT INTO operation_task_output (task_uuid, store_uuid) VALUES (?, ?)`, taskUUID, storeUUID)
	s.addMetadataStorePath(c, storeUUID, path)
	return storeUUID
}

// addMetadataStorePath links a store metadata to a path
func (s *baseSuite) addMetadataStorePath(c *tc.C, storeUUID, path string) {
	s.query(c, `INSERT INTO object_store_metadata_path (path, metadata_uuid) VALUES (?, ?)`, path, storeUUID)
}

// addOperationTaskOutput links a task to an object store metadata, return the
// store metadata uuid
func (s *baseSuite) addOperationTaskOutput(c *tc.C, taskUUID string) string {
	storeUUID := s.addFakeMetadataStore(c, 42)
	s.query(c, `INSERT INTO operation_task_output (task_uuid, store_uuid) VALUES (?, ?)`, taskUUID, storeUUID)
	return storeUUID
}

// addOperationTaskStatus sets a status for the task with the given textual status name.
func (s *baseSuite) addOperationTaskStatus(c *tc.C, taskUUID, status string) {
	beforeCount := s.getRowCount(c, "operation_task_status")
	// Map status to id via the table
	s.query(c, `INSERT INTO operation_task_status (task_uuid, status_id, updated_at) 
		SELECT ?, id, ? FROM operation_task_status_value WHERE status = ?`, taskUUID, s.state.clock.Now(), status)
	afterCount := s.getRowCount(c, "operation_task_status")
	c.Assert(afterCount, tc.Equals, beforeCount+1, tc.Commentf("status %q is not valid, is any of %v", status,
		s.selectDistinctValues(c, "status", "operation_task_status_value")))
}

// addOperationTaskLog inserts a log message for a task.
func (s *baseSuite) addOperationTaskLog(c *tc.C, taskUUID, content string) {
	s.query(c, `INSERT INTO operation_task_log (task_uuid, content, created_at) VALUES (?, ?, ?)`,
		taskUUID, content, s.state.clock.Now().UTC())
}

// addOperationParameter inserts a parameter key/value for an operation.
func (s *baseSuite) addOperationParameter(c *tc.C, operationUUID, key, value string) {
	s.query(c, `INSERT INTO operation_parameter (operation_uuid, "key", value) VALUES (?, ?, ?)`, operationUUID, key, value)
}

// addApplication creates a new application and returns its UUID
func (s *startSuite) addApplication(c *tc.C, charmUUID, appName string) string {
	appUUID := internaluuid.MustNewUUID().String()
	s.query(c, `INSERT INTO application (uuid, name, life_id, charm_uuid, space_uuid) VALUES (?, ?, ?, ?, ?)`,
		appUUID, appName, 0, charmUUID, "656b4a82-e28c-53d6-a014-f0dd53417eb6")
	return appUUID
}

// addUnitToApplication creates a unit for an existing application
func (s *startSuite) addUnitToApplication(c *tc.C, charmUUID, appUUID, unitName string) string {
	nodeUUID := internaluuid.MustNewUUID().String()
	unitUUID := internaluuid.MustNewUUID().String()
	s.query(c, `INSERT INTO net_node (uuid) VALUES (?)`, nodeUUID)
	s.query(c, `INSERT INTO unit (uuid, name, life_id, application_uuid, charm_uuid, net_node_uuid) VALUES (?, ?, ?, ?, ?, ?)`,
		unitUUID, unitName, 0, appUUID, charmUUID, nodeUUID)
	return unitUUID
}
