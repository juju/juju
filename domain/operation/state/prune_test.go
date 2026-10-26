// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"fmt"
	"math"
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
	// Expect: At this point data other than stored one are negligible, so we
	// got 5KiB total and 2KiB avg (4KiB + neglictable data), rounded up to 5KiB.
	c.Check(total, tc.Equals, 5)
	c.Check(avg, tc.Equals, 5/2)
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
	// Expect: At this point we have about 5KiB of logs and 4KiB of object store
	c.Check(total, tc.Equals, 9)
	c.Check(avg, tc.Equals, 9/2)
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

// sizedTimestamp is a timestamp in text form, which is 20 bytes long.
const sizedTimestamp = "2024-01-02T03:04:05Z"

// TestComputeTableSizeByColumnType pins the number of bytes the estimate
// measures for each kind of value stored in the columns it covers: integers
// (zero, negative and large), booleans, timestamps stored as text, integers
// and reals, text and NULL. NULL values are not counted.
func (s *pruneBySizeSuite) TestComputeTableSizeByColumnType(c *tc.C) {
	// Arrange
	operationColumns := []string{"uuid", "operation_id", "summary", "enqueued_at", "started_at",
		"completed_at", "parallel", "execution_group"}
	s.insertSized(c, "operation", operationColumns,
		"v1", 0, "", int64(1704164645), sizedTimestamp, 1704164645.5, true, "")
	s.insertSized(c, "operation", operationColumns,
		"v2", -42, "héllo", sizedTimestamp, nil, nil, false, "é")
	s.insertSized(c, "operation", operationColumns,
		"v3", int64(math.MaxInt64), nil, 1704164645.25, int64(1704164646), sizedTimestamp, nil, nil)
	s.insertSized(c, "operation", operationColumns,
		"v4", 1700000000, "x", 0, -1, nil, nil, "g")

	taskColumns := []string{"uuid", "operation_uuid", "task_id", "enqueued_at", "started_at", "completed_at"}
	s.insertSized(c, "operation_task", taskColumns,
		"t1", "v1", "10", int64(1704164645), sizedTimestamp, 1704164645.5)
	s.insertSized(c, "operation_task", taskColumns,
		"t2", "v2", "11", sizedTimestamp, nil, 0)
	s.insertSized(c, "operation_task", taskColumns,
		"t3", "v3", "12", int64(1704164645), nil, nil)

	statusColumns := []string{"task_uuid", "status_id", "message", "updated_at"}
	s.insertSized(c, "operation_task_status", statusColumns, "t1", 0, "ok", sizedTimestamp)
	s.insertSized(c, "operation_task_status", statusColumns, "t2", 7, nil, int64(1704164645))

	logColumns := []string{"task_uuid", "content", "created_at"}
	s.insertSized(c, "operation_task_log", logColumns, "t1", "héllo", sizedTimestamp)
	s.insertSized(c, "operation_task_log", logColumns, "t1", "", 1704164645.25)
	s.insertSized(c, "operation_task_log", logColumns, "t2", "x", int64(1704164645))

	// Act and assert: each column is measured on its own, so a NULL only
	// affects its own column.
	for _, test := range []struct {
		table   string
		column  string
		nonText bool
		size    int
	}{
		{"operation", "uuid", false, 8},
		{"operation", "operation_id", true, 1 + 3 + 19 + 10},
		{"operation", "summary", false, 0 + 6 + 1},
		{"operation", "enqueued_at", true, 10 + 20 + 13 + 1},
		{"operation", "started_at", true, 20 + 10 + 2},
		{"operation", "completed_at", true, 12 + 20},
		{"operation", "parallel", true, 1 + 1},
		{"operation", "execution_group", false, 0 + 2 + 1},

		{"operation_task", "uuid", false, 6},
		{"operation_task", "task_id", false, 6},
		{"operation_task", "enqueued_at", true, 10 + 20 + 10},
		{"operation_task", "started_at", true, 20},
		{"operation_task", "completed_at", true, 12 + 1},

		{"operation_task_status", "status_id", true, 1 + 1},
		{"operation_task_status", "message", false, 2},
		{"operation_task_status", "updated_at", true, 20 + 10},

		{"operation_task_log", "content", false, 6 + 0 + 1},
		{"operation_task_log", "created_at", true, 20 + 13 + 10},
	} {
		c.Check(s.columnSize(c, test.table, test.column, test.nonText), tc.Equals, test.size,
			tc.Commentf("%s.%s", test.table, test.column))
	}
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

// TestEstimateOperationSizeExactBytes verifies that the estimate follows the
// exact number of bytes stored, by moving the stored size by a single byte
// around multiples of one KiB.
func (s *pruneBySizeSuite) TestEstimateOperationSizeExactBytes(c *tc.C) {
	f := s.addSizedFixture(c)

	for _, test := range []struct {
		bytes int
		total int
		avg   int
	}{
		{bytes: 4095, total: 4, avg: 1},
		{bytes: 4096, total: 4, avg: 2},
		{bytes: 4097, total: 5, avg: 2},
		{bytes: 6143, total: 6, avg: 2},
		{bytes: 6144, total: 6, avg: 3},
		{bytes: 6145, total: 7, avg: 3},
	} {
		f.setBytes(c, test.bytes)

		var total, avg int
		err := s.txn(c, func(ctx context.Context, tx *sqlair.TX) error {
			var err error
			total, avg, err = s.state.estimateOperationSizeInKiB(ctx, tx)
			return err
		})
		c.Assert(err, tc.ErrorIsNil)
		c.Check(total, tc.Equals, test.total, tc.Commentf("total for %d bytes", test.bytes))
		c.Check(avg, tc.Equals, test.avg, tc.Commentf("average for %d bytes", test.bytes))
	}
}

// TestPruneBySizeAtThreshold verifies that nothing is pruned when the size is
// one byte below the point where an operation needs to be pruned.
func (s *pruneBySizeSuite) TestPruneBySizeAtThreshold(c *tc.C) {
	// Arrange: 2046 KiB for a limit of 1 MiB, and two operations of 1023 KiB.
	f := s.addSizedFixture(c)
	f.setBytes(c, 2046*humanize.KiByte)

	// Act
	storePaths, err := s.state.pruneOperationsToKeepUnderSizeMiB(c.Context(), 1)

	// Assert
	c.Assert(err, tc.ErrorIsNil)
	c.Check(storePaths, tc.HasLen, 0)
	c.Check(s.selectDistinctValues(c, "uuid", "operation"), tc.SameContents, []string{f.oldOperation, f.newOperation})
}

// TestPruneBySizeJustOverThreshold verifies that the oldest operation is pruned
// when the size is one byte above the point where an operation needs to be
// pruned.
func (s *pruneBySizeSuite) TestPruneBySizeJustOverThreshold(c *tc.C) {
	// Arrange: 2047 KiB for a limit of 1 MiB, and two operations of 1023 KiB.
	f := s.addSizedFixture(c)
	f.setBytes(c, 2046*humanize.KiByte+1)

	// Act
	storePaths, err := s.state.pruneOperationsToKeepUnderSizeMiB(c.Context(), 1)

	// Assert: the operation completed first is pruned, with its output.
	c.Assert(err, tc.ErrorIsNil)
	c.Check(storePaths, tc.SameContents, []string{f.storePath})
	c.Check(s.selectDistinctValues(c, "uuid", "operation"), tc.SameContents, []string{f.newOperation})
}

// columnSize returns the size in bytes measured for a single column of a
// table.
func (s *pruneBySizeSuite) columnSize(c *tc.C, table, column string, nonText bool) int {
	col := textColumn(column)
	if nonText {
		col = nonTextColumn(column)
	}
	var size int
	err := s.txn(c, func(ctx context.Context, tx *sqlair.TX) error {
		var err error
		size, err = s.state.computeTableSize(ctx, tx, humanize.Byte, table, col)
		return err
	})
	c.Assert(err, tc.ErrorIsNil)
	return size
}

// sizedFixture describes a set of rows of known size added by addSizedFixture.
type sizedFixture struct {
	s *pruneBySizeSuite
	// oldOperation is completed, newOperation is not, so oldOperation is the
	// first to be pruned.
	oldOperation string
	newOperation string
	// storePath is the path of the object store entry of the old operation.
	storePath string
	// bytes is the size of the rows, without any padding.
	bytes int
}

// addSizedFixture adds two operations with rows in each of the tables covered
// by the size estimate, using the different kinds of values stored in them. The
// operation which is not completed has NULL columns, so it is not counted in
// the size of the operation table, but its other rows are.
func (s *pruneBySizeSuite) addSizedFixture(c *tc.C) *sizedFixture {
	f := &sizedFixture{
		s:            s,
		oldOperation: "op-old",
		newOperation: "op-new",
		storePath:    "/out/path",
	}

	operationColumns := []string{"uuid", "operation_id", "summary", "enqueued_at", "started_at",
		"completed_at", "parallel", "execution_group"}
	f.bytes += s.insertSized(c, "operation", operationColumns,
		f.oldOperation, int64(math.MaxInt64), "old summary", sizedTimestamp, int64(1704164646),
		sizedTimestamp, true, "é-group")
	f.bytes += s.insertSized(c, "operation", operationColumns,
		f.newOperation, -7, "", 1704164645.5, nil, nil, false, "")

	taskColumns := []string{"uuid", "operation_uuid", "task_id", "enqueued_at", "started_at", "completed_at"}
	f.bytes += s.insertSized(c, "operation_task", taskColumns,
		"task-old", f.oldOperation, "100", sizedTimestamp, int64(1704164645), 1704164645.5)
	f.bytes += s.insertSized(c, "operation_task", taskColumns,
		"task-new", f.newOperation, "101", 1704164645.25, nil, nil)

	statusColumns := []string{"task_uuid", "status_id", "message", "updated_at"}
	f.bytes += s.insertSized(c, "operation_task_status", statusColumns, "task-old", 5, "done", sizedTimestamp)
	f.bytes += s.insertSized(c, "operation_task_status", statusColumns, "task-new", 0, "", int64(1704164650))

	logColumns := []string{"task_uuid", "content", "created_at"}
	f.bytes += s.insertSized(c, "operation_task_log", logColumns, "task-old", "line", sizedTimestamp)
	f.bytes += s.insertSized(c, "operation_task_log", logColumns, "task-old", "", int64(1704164651))
	f.bytes += s.insertSized(c, "operation_task_log", logColumns, "task-new", "héllo", 1704164651.25)

	charmUUID := s.addCharm(c)
	s.query(c, `INSERT INTO charm_action (charm_uuid, "key") VALUES (?, ?)`, charmUUID, "act")
	f.bytes += s.insertSized(c, "operation_action", []string{"operation_uuid", "charm_uuid", "charm_action_key"},
		f.oldOperation, charmUUID, "act")

	parameterColumns := []string{"operation_uuid", "key", "value"}
	f.bytes += s.insertSized(c, "operation_parameter", parameterColumns, f.oldOperation, "key", "value")
	f.bytes += s.insertSized(c, "operation_parameter", parameterColumns, f.oldOperation, "other", `"héllo"`)

	// A unit and a machine task, and an output, are each counted as two UUIDs.
	s.addOperationMachineTask(c, "task-old", s.addMachine(c, "0"))
	s.addOperationUnitTask(c, "task-new", s.addUnit(c, charmUUID))
	f.bytes += 3 * 2 * uuidSizeInB

	const (
		sha256 = "sha256-value"
		sha384 = "sha384-value"
		size   = 3000
	)
	storeUUID := s.addOperationTaskOutputWithData(c, "task-old", sha256, sha384, size, f.storePath)
	f.bytes += len("task-old") + 2*len(f.storePath) + len(storeUUID)
	f.bytes += size + len(storeUUID) + len(sha256) + len(sha384) + len(strconv.Itoa(size))
	return f
}

// setBytes adds an operation parameter which brings the size of the rows to the
// given number of bytes.
func (f *sizedFixture) setBytes(c *tc.C, bytes int) {
	const keyAndOperationSize = len("op-old") + len("pad")
	padding := bytes - f.bytes - keyAndOperationSize
	c.Assert(padding >= 0, tc.IsTrue, tc.Commentf("fixture of %d bytes is larger than %d", f.bytes, bytes))
	f.s.query(c, `DELETE FROM operation_parameter WHERE "key" = 'pad'`)
	f.s.insertSized(c, "operation_parameter", []string{"operation_uuid", "key", "value"},
		f.oldOperation, "pad", strings.Repeat("p", padding))
}

// insertSized inserts a row with the given values, and returns the number of
// bytes the row is expected to contribute to the size estimate.
func (s *pruneBySizeSuite) insertSized(c *tc.C, table string, columns []string, values ...any) int {
	c.Assert(values, tc.HasLen, len(columns))
	quoted := make([]string, len(columns))
	placeholders := make([]string, len(columns))
	for i, column := range columns {
		quoted[i] = strconv.Quote(column)
		placeholders[i] = "?"
	}
	s.query(c, fmt.Sprintf(`INSERT INTO %q (%s) VALUES (%s)`,
		table, strings.Join(quoted, ", "), strings.Join(placeholders, ", ")), values...)
	return rowSize(c, values)
}

// rowSize returns the size of a row of values, which is the sum of the sizes of
// the text form of each value. The size of a row with a NULL value is NULL,
// which is not counted when summing the size of the rows of a table.
func rowSize(c *tc.C, values []any) int {
	var size int
	for _, value := range values {
		switch v := value.(type) {
		case nil:
			return 0
		case string:
			size += len(v)
		case int:
			size += len(strconv.Itoa(v))
		case int64:
			size += len(strconv.FormatInt(v, 10))
		case float64:
			size += len(strconv.FormatFloat(v, 'f', -1, 64))
		case bool:
			// Booleans are stored as 0 or 1.
			size++
		default:
			c.Fatalf("unsupported value %v of type %T", value, value)
		}
	}
	return size
}

// generateLongString creates and returns a long string containing the given
// number of characters.
// This is used to generate long strings for logs and object store.
func (s *pruneBySizeSuite) generateLongString(c *tc.C, size int) string {
	return strings.Repeat("a", size)
}
