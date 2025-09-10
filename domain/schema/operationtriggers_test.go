// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package schema

import (
	"testing"

	"github.com/juju/tc"

	corestatus "github.com/juju/juju/core/status"
	"github.com/juju/juju/internal/uuid"
)

// operationSuite is a set of tests for asserting the behaviour of the
// storage provisioning triggers that exist in the model schema.
type operationSuite struct {
	schemaBaseSuite
}

// TestOperationSuite registers the tests for the [operationSuite].
func TestOperationSuite(t *testing.T) {
	tc.Run(t, &operationSuite{})
}

// SetUpTest is responsible for setting up the model DDL so the operation
// triggers can be tested.
func (s *operationSuite) SetUpTest(c *tc.C) {
	s.schemaBaseSuite.SetUpTest(c)
	s.applyDDL(c, ModelDDL())
}

// TestOperationTaskStatusAborting Trigger tests that changing the status to
// ABORTING, triggers a change log event.
func (s *operationSuite) TestOperationTaskStatusAbortingTrigger(c *tc.C) {
	// Arrange
	taskUUID := s.newOperationTaskStatus(c, "first", corestatus.Pending)

	// Act
	s.updateOperationTaskStatus(c, taskUUID, "second", corestatus.Aborting)

	// Assert
	s.assertChangeEvent(c, "custom_operation_task_status_aborting", taskUUID)
}

// TestOperationTaskStatusAbortingNotTriggered tests that changing the status to
// a value other than ABORTING, does not trigger a change log event.
func (s *operationSuite) TestOperationTaskStatusAbortingNotTriggered(c *tc.C) {
	// Arrange
	taskUUID := s.newOperationTaskStatus(c, "first", corestatus.Pending)

	// Act
	s.updateOperationTaskStatus(c, taskUUID, "second", corestatus.Completed)

	// Assert
	s.assertChangeEventCountNoType(c, "custom_operation_task_status_aborting", taskUUID, 0)
}

// TestOperationTaskStatusAbortingNotTriggered tests deleting the status row,
// does not trigger a change log event.
func (s *operationSuite) TestOperationTaskStatusAbortingTriggeredStatusDeleted(c *tc.C) {
	// Arrange
	taskUUID := s.newOperationTaskStatus(c, "first", corestatus.Pending)

	// Act
	s.deleteOperationTaskStatus(c, taskUUID)

	// Assert
	s.assertChangeEventCountNoType(c, "custom_operation_task_status_aborting", taskUUID, 0)
}

func (s *operationSuite) newOperationTaskStatus(c *tc.C, msg string, status corestatus.Status) string {
	operationUUID := uuid.MustNewUUID().String()
	taskUUID := uuid.MustNewUUID().String()

	_, err := s.DB().ExecContext(
		c.Context(),
		`
INSERT INTO operation (uuid, operation_id, enqueued_at)
VALUES (?, ?, DATETIME('now'))
`,
		operationUUID, s.nextSeq())
	c.Assert(err, tc.ErrorIsNil)

	_, err = s.DB().ExecContext(
		c.Context(),
		`
INSERT INTO operation_task (uuid, operation_uuid, task_id, enqueued_at)
VALUES (?, ?, ?, DATETIME('now'))
`,
		taskUUID, operationUUID, s.nextSeq())
	c.Assert(err, tc.ErrorIsNil)

	_, err = s.DB().ExecContext(
		c.Context(),
		`
INSERT INTO operation_task_status (task_uuid, status_id, message, updated_at)
SELECT ?, otsv.id, ?, DATETIME('now')
FROM   operation_task_status_value AS otsv
WHERE  otsv.status = ?
`,
		taskUUID, msg, status)
	c.Assert(err, tc.ErrorIsNil)

	return taskUUID
}

func (s *operationSuite) updateOperationTaskStatus(c *tc.C, taskUUID, msg string, status corestatus.Status) {
	_, err := s.DB().ExecContext(
		c.Context(),
		`
UPDATE operation_task_status
SET    status_id = (SELECT id FROM operation_task_status_value WHERE status = ?),
       message = ?,
       updated_at = DATETIME('now')
WHERE  task_uuid = ?
`,
		status, msg, taskUUID)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *operationSuite) deleteOperationTaskStatus(c *tc.C, taskUUID string) {
	_, err := s.DB().ExecContext(
		c.Context(),
		`
DELETE FROM operation_task_status WHERE task_uuid = ?
`,
		taskUUID)
	c.Assert(err, tc.ErrorIsNil)
}

// assertChangeEvent asserts that the requested number of change event exists
// for the provided namespace and changed value.
func (s *operationSuite) assertChangeEventCountNoType(
	c *tc.C, namespace string, changed string, expectedCount int,
) {
	nsID := s.getNamespaceID(c, namespace)

	row := s.DB().QueryRow(`
SELECT COUNT(*)
FROM   change_log
WHERE  namespace_id = ?
AND    changed = ?`, nsID, changed)
	var count int
	err := row.Scan(&count)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(count, tc.Equals, expectedCount)
}
