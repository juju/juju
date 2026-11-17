// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/canonical/sqlair"
	"github.com/dustin/go-humanize"
	"github.com/juju/tc"
)

type pruneByAgeSuite struct {
	baseSuite
}

type pruneBySizeSuite struct {
	baseSuite
}

func TestPruneByAgeSuite(t *testing.T) {
	tc.Run(t, &pruneByAgeSuite{})
}

func TestPruneBySizeSuite(t *testing.T) {
	tc.Run(t, &pruneBySizeSuite{})
}

// TestPruneCompletedOperationsOlderThan tests that the prune completed operation
// by age function deletes the completed operations.
func (s *pruneByAgeSuite) TestPruneCompletedOperationsOlderThan(c *tc.C) {
	// Arrange: three operation, on is not completed, one is recently completed,
	// on need to be deleted by the prune.
	toDeleteOperation := s.addCompletedOperation(c, time.Minute)
	controlCompleted := s.addCompletedOperation(c, time.Second)
	controlNotCompleted := s.addOperation(c)
	s.addOperationTaskOutputWithPath(c, s.addOperationTask(c, toDeleteOperation), "path/to/test")

	// Act: prune all completed operation older than 30 sec.
	storeUUIDs, err := s.state.pruneCompletedOperationsOlderThan(c.Context(), 30*time.Second)

	// Assert: the operation is deleted.
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(storeUUIDs, tc.SameContents, []string{"path/to/test"})
	c.Check(s.selectDistinctValues(c, "uuid", "operation"), tc.SameContents, []string{controlNotCompleted, controlCompleted})
}

// TestGetCompletedOperationsOlderThan tests that the get completed operations
// by age function returns the completed operations.
func (s *pruneByAgeSuite) TestGetCompletedOperationsOlderThan(c *tc.C) {
	// Arrange: three operation, on is not completed, one is recently completed,
	// on need to be deleted by the prune.
	completed := s.addCompletedOperation(c, time.Minute)
	controlCompleted := s.addCompletedOperation(c, time.Second)
	controlNotCompleted := s.addOperation(c)

	// Act: get old completed operations.
	var opUUIDs []string
	err := s.txn(c, func(ctx context.Context, tx *sqlair.TX) error {
		var err error
		opUUIDs, err = s.state.getCompletedOperationUUIDsOlderThan(ctx, tx, 30*time.Second)
		return err
	})

	// Assert: the operation is deleted.
	c.Assert(err, tc.ErrorIsNil)
	c.Check(opUUIDs, tc.DeepEquals, []string{completed})
	c.Check(s.selectDistinctValues(c, "uuid", "operation"), tc.SameContents, []string{controlNotCompleted,
		controlCompleted, completed})
}

// TestGetCompletedOperationOlderThanNegativeAge tests that the get completed
// operation by age function returns no results when the age is negative.
func (s *pruneByAgeSuite) TestGetCompletedOperationOlderThanNegativeAge(c *tc.C) {
	// Arrange
	s.addCompletedOperation(c, time.Minute)

	// Act
	var opUUIDs []string
	err := s.txn(c, func(ctx context.Context, tx *sqlair.TX) error {
		var err error
		opUUIDs, err = s.state.getCompletedOperationUUIDsOlderThan(ctx, tx, -30*time.Second)
		return err
	})
	// Assert: no error, no results
	c.Assert(err, tc.ErrorIsNil)
	c.Check(opUUIDs, tc.HasLen, 0)
}

// TestGetCompletedOperationOlderThanZeroAge verifies that no completed
// operations are returned when the age is zero.
func (s *pruneByAgeSuite) TestGetCompletedOperationOlderThanZeroAge(c *tc.C) {
	// Arrange
	s.addCompletedOperation(c, time.Minute)

	// Act
	var opUUIDs []string
	err := s.txn(c, func(ctx context.Context, tx *sqlair.TX) error {
		var err error
		opUUIDs, err = s.state.getCompletedOperationUUIDsOlderThan(ctx, tx, 0)
		return err
	})
	// Assert: no error, no results
	c.Assert(err, tc.ErrorIsNil)
	c.Check(opUUIDs, tc.HasLen, 0)
}

// TestGetOperationToPruneUpToNoOperation tests that the get operation to prune
// up to function returns no operation without errors when there is no
// operation in the database.
func (s *pruneBySizeSuite) TestGetOperationToPruneUpToNoOperation(c *tc.C) {
	// Arrange: no operation in the database.

	// Act: get operation to prune up to.
	var opUUIDs []string
	err := s.txn(c, func(ctx context.Context, tx *sqlair.TX) error {
		var err error
		opUUIDs, err = s.state.getOperationToPruneUpTo(ctx, tx, 100)
		return err
	})

	// Assert: no error, no operation to prune.
	c.Assert(err, tc.ErrorIsNil)
	c.Check(opUUIDs, tc.HasLen, 0)
}

// TestGetOperationToPruneUpToLessOperationThanRequired tests that the get
// operation to prune up to function returns the operation to prune when there
// is less operation than the required number of operation.
func (s *pruneBySizeSuite) TestGetOperationToPruneUpToLessOperationThanRequired(c *tc.C) {
	// Arrange: few operations in the database.
	opsToPrune := []string{
		s.addOperation(c),
		s.addCompletedOperation(c, time.Minute), // completed one minute ago
		s.addOperation(c),
	}

	// Act: get operation to prune up to.
	var opUUIDs []string
	err := s.txn(c, func(ctx context.Context, tx *sqlair.TX) error {
		var err error
		opUUIDs, err = s.state.getOperationToPruneUpTo(ctx, tx, 100)
		return err
	})

	// Assert: no error, all operations retrieved
	c.Assert(err, tc.ErrorIsNil)
	c.Check(opUUIDs, tc.SameContents, opsToPrune)

}

// TestGetOperationToPruneUpToLessOperationMoreThanRequired tests that the get
// operation to prune up to function returns prioritized operation when there is
// more operation than the required number of operation.
func (s *pruneBySizeSuite) TestGetOperationToPruneUpToLessOperationMoreThanRequired(c *tc.C) {
	// Arrange: few operations in the database.

	op1 := s.addOperation(c)
	op2 := s.addCompletedOperation(c, time.Minute)
	s.addOperation(c) // discarded (complete are priorized, and more recent than op1)
	op3 := s.addCompletedOperation(c, 2*time.Minute)

	// Act: get operation to prune up to.
	var opUUIDs []string
	err := s.txn(c, func(ctx context.Context, tx *sqlair.TX) error {
		var err error
		opUUIDs, err = s.state.getOperationToPruneUpTo(ctx, tx, 3)
		return err
	})

	// Assert: no error, all operations retrieved
	c.Assert(err, tc.ErrorIsNil)
	c.Check(opUUIDs, tc.SameContents, []string{op1, op2, op3})
}

// TestGetOperationToPruneUpTo tests that the get operation to prune up to
// function returns prioritized operation when there is more operation than the
// required number of operation.
func (s *pruneBySizeSuite) TestGetOperationToPruneUpTo(c *tc.C) {
	// Arrange: few operations in the database.

	op1 := s.addCompletedOperation(c, time.Hour)
	s.addCompletedOperation(c, time.Second) // discarded (more recent)
	op2 := s.addCompletedOperation(c, time.Minute)
	op3 := s.addCompletedOperation(c, 10*time.Minute)
	s.addOperation(c) // discarded (complete are priorized)

	// Act: get operation to prune up to.
	var opUUIDs []string
	err := s.txn(c, func(ctx context.Context, tx *sqlair.TX) error {
		var err error
		opUUIDs, err = s.state.getOperationToPruneUpTo(ctx, tx, 3)
		return err
	})

	// Assert: no error, all operations retrieved
	c.Assert(err, tc.ErrorIsNil)
	c.Check(opUUIDs, tc.SameContents, []string{op1, op2, op3})
}

// TestComputeObjectStoreSizeNoOutputs verifies size is zero when no outputs exist.
func (s *pruneBySizeSuite) TestComputeObjectStoreSizeNoOutputs(c *tc.C) {
	var size int
	err := s.txn(c, func(ctx context.Context, tx *sqlair.TX) error {
		var err error
		size, err = s.state.computeObjectStoreSize(ctx, tx, 1)
		return err
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(size, tc.Equals, 0)
}

// TestComputeObjectStoreSizeSumsOnlyReferenced ensures only sizes referenced by operation_task_output are summed.
func (s *pruneBySizeSuite) TestComputeObjectStoreSizeSumsOnlyReferenced(c *tc.C) {
	// Arrange: create an operation and two tasks with outputs, plus an unreferenced object store entry.
	opUUID := s.addOperation(c)
	task1 := s.addOperationTask(c, opUUID)
	task2 := s.addOperationTask(c, opUUID)
	// Referenced outputs
	s.addOperationTaskOutputWithData(c, task1, "sha1", "shaA", 1*humanize.KiByte, "/path/a")
	s.addOperationTaskOutputWithData(c, task2, "sha2", "shaB", 3*humanize.KiByte, "/path/b")
	// Unreferenced object store metadata, should be ignored by computeObjectStoreSize
	s.addFakeMetadataStore(c, 999999)

	var size int
	err := s.txn(c, func(ctx context.Context, tx *sqlair.TX) error {
		var err error
		// Expect: size = 1KiB + 3KiB + (data from other fields) -> 5 KiB (roundedUp)
		size, err = s.state.computeObjectStoreSize(ctx, tx, humanize.KiByte)
		return err
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(size, tc.Equals, 5)
}

// TestComputeObjectStoreSizeConvertsToKiB verifies conversion using humanize.KiByte.
func (s *pruneBySizeSuite) TestComputeObjectStoreSizeConvertsToKiB(c *tc.C) {
	// Arrange
	opUUID := s.addOperation(c)
	task := s.addOperationTask(c, opUUID)
	// Add output of 2078 bytes so 2078/1024 = 3 (integer division, rounded up) KiB
	s.addOperationTaskOutputWithData(c, task, "sha3", "shaC", 2078, "/path/c")

	var sizeKiB int
	err := s.txn(c, func(ctx context.Context, tx *sqlair.TX) error {
		var err error
		sizeKiB, err = s.state.computeObjectStoreSize(ctx, tx, humanize.KiByte)
		return err
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(sizeKiB, tc.Equals, 3)
}

// TestEstimateOperationSizeEmptyDB ensures that when there are no operations,
// the function returns total=0 and avg=-1 without error.
func (s *pruneBySizeSuite) TestEstimateOperationSizeEmptyDB(c *tc.C) {
	var total, avg int
	err := s.txn(c, func(ctx context.Context, tx *sqlair.TX) error {
		var err error
		total, avg, err = s.state.estimateOperationSizeInKiB(ctx, tx)
		return err
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(total, tc.Equals, 0)
	c.Check(avg, tc.Equals, -1)
}

// TestEstimateOperationSizeWithData verifies that row counts and object store
// sizes are combined using the row size factors and KiB rounding for object store.
func (s *pruneBySizeSuite) TestEstimateOperationSizeWithData(c *tc.C) {
	// Arrange: 2 operations, 3 tasks, 5 logs, object store total 4000 bytes -> 4 KiB.
	op1 := s.addOperation(c)
	op2 := s.addOperation(c)
	// Tasks
	t1 := s.addOperationTask(c, op1)
	t2 := s.addOperationTask(c, op1)
	t3 := s.addOperationTask(c, op2)
	// Logs (5 entries total)
	s.addOperationTaskLog(c, t1, "log1")
	s.addOperationTaskLog(c, t1, "log2")
	s.addOperationTaskLog(c, t2, "log3")
	s.addOperationTaskLog(c, t3, "log4")
	s.addOperationTaskLog(c, t3, "log5")
	// Object store referenced by two outputs: 1KiB + 3KiB = 4KiB
	s.addOperationTaskOutputWithData(c, t1, "shaA", "shaA3", 1*humanize.KiByte, "/a")
	s.addOperationTaskOutputWithData(c, t2, "shaB", "shaB3", 3*humanize.KiByte, "/b")

	var total, avg int
	err := s.txn(c, func(ctx context.Context, tx *sqlair.TX) error {
		var err error
		total, avg, err = s.state.estimateOperationSizeInKiB(ctx, tx)
		return err
	})
	c.Assert(err, tc.ErrorIsNil)
	// Nullable columns contribute zero, while the rest of each row is counted.
	// The bytes beyond the 4 KiB object store round the total up to 6 KiB.
	c.Check(total, tc.Equals, 6)
	c.Check(avg, tc.Equals, 2)
}

// TestEstimateOperationSizeWithDataSmallTasks verifies that row counts and object store
// sizes are combined using the row size factors and KiB rounding for object store.
// This test is similar to TestEstimateOperationSizeWithData, but with only 2 tasks.
// The difference is that the first task has no outputs, so it is not counted in the
// object store size.
func (s *pruneBySizeSuite) TestEstimateOperationSizeWithDataSmallTasks(c *tc.C) {
	// Arrange: 2 operations, 3 tasks, object store total 0 bytes -> 1 KiB.
	op1 := s.addOperation(c)
	op2 := s.addOperation(c)
	// Tasks
	t1 := s.addOperationTask(c, op1)
	t2 := s.addOperationTask(c, op1)
	s.addOperationTask(c, op2)
	// Object store referenced by two outputs: 1KiB + 3KiB = 4KiB
	s.addOperationTaskOutputWithData(c, t1, "shaA", "shaA3", 0, "/a")
	s.addOperationTaskOutputWithData(c, t2, "shaB", "shaB3", 0, "/b")

	var total, avg int
	err := s.txn(c, func(ctx context.Context, tx *sqlair.TX) error {
		var err error
		total, avg, err = s.state.estimateOperationSizeInKiB(ctx, tx)
		return err
	})
	c.Assert(err, tc.ErrorIsNil)
	// Expect: size of data is zero, so we rely on the actual value in the database.
	// There is no logs, so it is less that 1 KiB for 2 operations.
	c.Check(total, tc.Equals, 1)
	c.Check(avg, tc.Equals, 1) // rounder up to 1
}

func (s *pruneBySizeSuite) TestEstimateOperationSizeWithDataBigLog(c *tc.C) {
	// Arrange:
	op1 := s.addOperation(c)
	op2 := s.addOperation(c)
	// Tasks
	t1 := s.addOperationTask(c, op1)
	t2 := s.addOperationTask(c, op1)
	t3 := s.addOperationTask(c, op2)
	// Logs (5 entries total)
	s.addOperationTaskLog(c, t1, s.generateLongString(c, humanize.KiByte/2))
	s.addOperationTaskLog(c, t1, s.generateLongString(c, humanize.KiByte/3))
	s.addOperationTaskLog(c, t2, s.generateLongString(c, humanize.KiByte/4))
	s.addOperationTaskLog(c, t3, s.generateLongString(c, 1*humanize.KiByte))
	s.addOperationTaskLog(c, t3, s.generateLongString(c, 2*humanize.KiByte))
	// Object store referenced by two outputs: 1KiB + 3KiB = 4KiB
	s.addOperationTaskOutputWithData(c, t1, "shaA", "shaA3", 1*humanize.KiByte, "/a")
	s.addOperationTaskOutputWithData(c, t2, "shaB", "shaB3", 3*humanize.KiByte, "/b")

	var total, avg int
	err := s.txn(c, func(ctx context.Context, tx *sqlair.TX) error {
		var err error
		total, avg, err = s.state.estimateOperationSizeInKiB(ctx, tx)
		return err
	})
	c.Assert(err, tc.ErrorIsNil)
	// Nullable columns contribute zero, while the rest of each row is counted.
	// The bytes beyond the logs and 4 KiB object store round up to 10 KiB.
	c.Check(total, tc.Equals, 10)
	c.Check(avg, tc.Equals, 4)
}

// TestPruneOperationsToKeepUnderSizeMiBIgnoresNonPositive ensures no pruning occurs when size Limit is zero or negative.
func (s *pruneBySizeSuite) TestPruneOperationsToKeepUnderSizeMiBIgnoresNonPositive(c *tc.C) {
	// Arrange: add some operations to ensure there is data.
	op1 := s.addOperation(c)
	op2 := s.addOperation(c)

	// Act: call with zero and negative; both should be no-ops and return nil.
	_, err := s.state.pruneOperationsToKeepUnderSizeMiB(c.Context(), 0)
	c.Assert(err, tc.ErrorIsNil)
	_, err = s.state.pruneOperationsToKeepUnderSizeMiB(c.Context(), -5)
	c.Assert(err, tc.ErrorIsNil)

	// Assert: operations unchanged
	c.Check(s.selectDistinctValues(c, "uuid", "operation"), tc.SameContents, []string{op1, op2})
}

// TestPruneOperationsToKeepUnderSizeMiBNoPruneWhenUnderLimit verifies no deletions when total size <= 1 MiB.
func (s *pruneBySizeSuite) TestPruneOperationsToKeepUnderSizeMiBNoPruneWhenUnderLimit(c *tc.C) {
	// Arrange: 2 operations, minimal data so total size is very small (<< 1 MiB).
	op1 := s.addOperation(c)
	op2 := s.addOperation(c)

	// Act
	_, err := s.state.pruneOperationsToKeepUnderSizeMiB(c.Context(), 1)
	c.Assert(err, tc.ErrorIsNil)

	// Assert: nothing deleted
	c.Check(s.selectDistinctValues(c, "uuid", "operation"), tc.SameContents, []string{op1, op2})
}

// TestPruneOperationsToKeepUnderSizeMiBPrunesExpected ensures pruning deletes the correct number of oldest/prioritized operations.
func (s *pruneBySizeSuite) TestPruneOperationsToKeepUnderSizeMiBPrunesExpected(c *tc.C) {
	// Arrange:
	// Create two operations: one completed long ago (opOld) and one not completed (opNew).
	opOld := s.addCompletedOperation(c, time.Hour)
	opNew := s.addOperation(c)
	// Add a task and large output to push total size well over 1 MiB.
	task := s.addOperationTask(c, opOld)
	// 3072 KiB worth of bytes
	bytes := 3072 * humanize.KiByte
	s.addOperationTaskOutputWithData(c, task, "shaX", "shaY", bytes, "/big")

	// Sanity: we now have object store ~3072 KiB + 2 ops + 1 task => total ~3075 KiB.
	// For max=1 MiB (1024 KiB), deletion count should be 1 based on average size.

	// Act
	storeUUIDs, err := s.state.pruneOperationsToKeepUnderSizeMiB(c.Context(), 1)
	c.Assert(err, tc.ErrorIsNil)

	// Assert: exactly one operation should remain: the newer one (opNew). The older completed opOld is deleted first.
	c.Check(storeUUIDs, tc.SameContents, []string{"/big"})
	c.Check(s.selectDistinctValues(c, "uuid", "operation"), tc.SameContents, []string{opNew})
}

// TestComputeObjectStoreSizeByValue pins the number of bytes measured for the
// object store entries referenced by task outputs, for sizes of zero, negative
// and larger than 32 bits.
func (s *pruneBySizeSuite) TestComputeObjectStoreSizeByValue(c *tc.C) {
	// Arrange
	opUUID := s.addOperation(c)
	var expected int
	for i, size := range []int{0, -3, 3000000000} {
		task := s.addOperationTask(c, opUUID)
		path := fmt.Sprintf("/path/%d", i)
		sha256 := fmt.Sprintf("sha256-%d", i)
		sha384 := fmt.Sprintf("sha384-%d", i)
		storeUUID := s.addOperationTaskOutputWithData(c, task, sha256, sha384, size, path)

		// Size of the operation_task_output row and of the metadata path row.
		expected += len(task) + 2*len(path) + len(storeUUID)
		// Size of the metadata row: the content size, followed by the size of
		// the text form of each column.
		expected += size + len(storeUUID) + len(sha256) + len(sha384) + len(strconv.Itoa(size))
	}

	// Act
	var obtained int
	err := s.txn(c, func(ctx context.Context, tx *sqlair.TX) error {
		var err error
		obtained, err = s.state.computeObjectStoreSize(ctx, tx, humanize.Byte)
		return err
	})

	// Assert
	c.Assert(err, tc.ErrorIsNil)
	c.Check(obtained, tc.Equals, expected)
}

// TestEstimateOperationSizeNullableFields verifies that a NULL field does not
// hide variable-length content elsewhere in the same row.
func (s *pruneBySizeSuite) TestEstimateOperationSizeNullableFields(c *tc.C) {
	op := s.addOperation(c)
	task := s.addOperationTask(c, op)
	s.query(c, `UPDATE operation SET summary = NULL, execution_group = ? WHERE uuid = ?`,
		strings.Repeat("é", 1024), op)
	s.query(c, `INSERT INTO operation_task_status (task_uuid, status_id, message, updated_at)
		VALUES (?, 0, ?, NULL)`, task, strings.Repeat("x", 2048))

	var total, avg int
	err := s.txn(c, func(ctx context.Context, tx *sqlair.TX) error {
		var err error
		total, avg, err = s.state.estimateOperationSizeInKiB(ctx, tx)
		return err
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(total, tc.Equals, 5)
	c.Check(avg, tc.Equals, 4)
}

// TestEstimateOperationSizeVariableContent covers each variable-length field
// and ensures multiple logs and parameters do not multiply one another.
func (s *pruneBySizeSuite) TestEstimateOperationSizeVariableContent(c *tc.C) {
	op := s.addOperation(c)
	task := s.addOperationTask(c, op)
	charm := s.addCharm(c)
	content := strings.Repeat("x", 4*humanize.KiByte)
	s.query(c, `UPDATE operation SET summary = ?, execution_group = ? WHERE uuid = ?`,
		content, content, op)
	s.query(c, `UPDATE operation_task SET task_id = ? WHERE uuid = ?`, content, task)
	s.addOperationTaskLog(c, task, content)
	s.addOperationTaskLog(c, task, content)
	s.addOperationParameter(c, op, "first", content)
	s.addOperationParameter(c, op, "second", content)
	s.addOperationAction(c, op, charm, content)
	s.query(c, `INSERT INTO operation_task_status (task_uuid, status_id, message)
		VALUES (?, 0, ?)`, task, content)

	var total, avg int
	err := s.txn(c, func(ctx context.Context, tx *sqlair.TX) error {
		var err error
		total, avg, err = s.state.estimateOperationSizeInKiB(ctx, tx)
		return err
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(total, tc.Equals, 37)
	c.Check(avg, tc.Equals, 36)
}

// TestEstimateOperationSizeSchema requires a review of the estimate whenever
// the columns or their declared types change.
func (s *pruneBySizeSuite) TestEstimateOperationSizeSchema(c *tc.C) {
	tables := map[string][]string{
		"operation": {
			"uuid TEXT", "operation_id INTEGER", "summary TEXT",
			"enqueued_at TIMESTAMP", "started_at TIMESTAMP", "completed_at TIMESTAMP",
			"parallel BOOLEAN", "execution_group TEXT",
		},
		"operation_task": {
			"uuid TEXT", "operation_uuid TEXT", "task_id TEXT",
			"enqueued_at DATETIME", "started_at DATETIME", "completed_at DATETIME",
		},
		"operation_task_log":         {"task_uuid TEXT", "content TEXT", "created_at TIMESTAMP"},
		"operation_action":           {"operation_uuid TEXT", "charm_uuid TEXT", "charm_action_key TEXT"},
		"operation_parameter":        {"operation_uuid TEXT", "key TEXT", "value TEXT"},
		"operation_task_status":      {"task_uuid TEXT", "status_id INT", "message TEXT", "updated_at DATETIME"},
		"operation_machine_task":     {"task_uuid TEXT", "machine_uuid TEXT"},
		"operation_unit_task":        {"task_uuid TEXT", "unit_uuid TEXT"},
		"operation_task_output":      {"task_uuid TEXT", "store_path TEXT"},
		"object_store_metadata":      {"uuid TEXT", "sha_256 TEXT", "sha_384 TEXT", "size INT"},
		"object_store_metadata_path": {"path TEXT", "metadata_uuid TEXT"},
	}
	for table, expected := range tables {
		rows := s.queryRows(c, `SELECT pti.name, pti.type
FROM pragma_table_info(?) AS pti ORDER BY pti.cid`, table)
		var columns []string
		for _, row := range rows {
			columns = append(columns, fmt.Sprintf("%s %s", row["name"], row["type"]))
		}
		c.Check(columns, tc.DeepEquals, expected,
			tc.Commentf("review the pruning estimate for table %s", table))
	}
}

// TestEstimateOperationSizeRepresentativeData compares the estimate against
// the byte lengths of known representative values and object content. These
// lengths do not include database pages, indexes or other storage overhead.
func (s *pruneBySizeSuite) TestEstimateOperationSizeRepresentativeData(c *tc.C) {
	const (
		timestamp = "2024-01-02T03:04:05Z"
		summary   = "example operation"
		group     = "group"
		message   = "status message"
		log       = "example log entry"
		action    = "action"
		key       = "parameter"
		value     = "example value"
		output    = 128
	)
	charm := s.addCharm(c)
	unit := s.addUnit(c, charm)
	machine := s.addMachine(c, "0")
	var referenceBytes int
	for i := range 12 {
		op := s.addOperation(c)
		task := s.addOperationTask(c, op)
		taskID := fmt.Sprintf("task-%d", i)
		s.query(c, `UPDATE operation SET operation_id = ?, summary = ?, execution_group = ?,
			enqueued_at = ?, started_at = ?, completed_at = ?, parallel = 1
			WHERE uuid = ?`, 1000+i, summary, group, timestamp, timestamp, timestamp, op)
		s.query(c, `UPDATE operation_task SET task_id = ?,
			enqueued_at = ?, started_at = ?, completed_at = ? WHERE uuid = ?`,
			taskID, timestamp, timestamp, timestamp, task)
		s.query(c, `INSERT INTO operation_task_status
			(task_uuid, status_id, message, updated_at) VALUES (?, 5, ?, ?)`,
			task, message, timestamp)
		s.query(c, `INSERT INTO operation_task_log (task_uuid, content, created_at)
			VALUES (?, ?, ?)`, task, log, timestamp)
		s.addOperationAction(c, op, charm, action)
		s.addOperationParameter(c, op, key, value)
		if i%2 == 0 {
			s.addOperationUnitTask(c, task, unit)
			referenceBytes += len(task) + len(unit)
		} else {
			s.addOperationMachineTask(c, task, machine)
			referenceBytes += len(task) + len(machine)
		}
		path := fmt.Sprintf("/output/%d", i)
		sha256 := fmt.Sprintf("sha256-%d", i)
		sha384 := fmt.Sprintf("sha384-%d", i)
		store := s.addOperationTaskOutputWithData(c, task, sha256, sha384, output, path)

		referenceBytes += len(op) + len(strconv.Itoa(1000+i)) + len(summary) +
			3*len(timestamp) + len("1") + len(group)
		referenceBytes += len(task) + len(op) + len(taskID) + 3*len(timestamp)
		referenceBytes += len(task) + len("5") + len(message) + len(timestamp)
		referenceBytes += len(task) + len(log) + len(timestamp)
		referenceBytes += len(op) + len(charm) + len(action)
		referenceBytes += len(op) + len(key) + len(value)
		referenceBytes += len(task) + 2*len(path) + 2*len(store) +
			len(sha256) + len(sha384) + len(strconv.Itoa(output)) + output
	}

	var total int
	err := s.txn(c, func(ctx context.Context, tx *sqlair.TX) error {
		var err error
		total, _, err = s.state.estimateOperationSizeInKiB(ctx, tx)
		return err
	})
	c.Assert(err, tc.ErrorIsNil)
	estimatedBytes := total * humanize.KiByte
	// Allow 25% error, plus one KiB for the estimate's rounding.
	c.Check(estimatedBytes >= referenceBytes*3/4, tc.IsTrue,
		tc.Commentf("estimated %d bytes; representative content %d bytes", estimatedBytes, referenceBytes))
	c.Check(estimatedBytes <= referenceBytes*5/4+humanize.KiByte, tc.IsTrue,
		tc.Commentf("estimated %d bytes; representative content %d bytes", estimatedBytes, referenceBytes))
}

// TestPruneBySizeCountsParameters verifies that large parameter content
// contributes to the decision to prune the oldest operation.
func (s *pruneBySizeSuite) TestPruneBySizeCountsParameters(c *tc.C) {
	old := s.addCompletedOperation(c, time.Hour)
	newer := s.addOperation(c)
	s.query(c, `INSERT INTO operation_parameter (operation_uuid, "key", value)
		VALUES (?, 'command', ?)`, old, strings.Repeat("x", 3*humanize.MiByte))

	paths, err := s.state.pruneOperationsToKeepUnderSizeMiB(c.Context(), 1)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(paths, tc.HasLen, 0)
	c.Check(s.selectDistinctValues(c, "uuid", "operation"), tc.SameContents, []string{newer})
}

// generateLongString creates and returns a long string containing the given
// number of characters.
// This is used to generate long strings for logs and object store.
func (s *pruneBySizeSuite) generateLongString(c *tc.C, size int) string {
	return strings.Repeat("a", size)
}
