// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/canonical/sqlair"
	"github.com/dustin/go-humanize"
	"github.com/juju/collections/transform"

	"github.com/juju/juju/internal/errors"
)

const (
	// UUID are stored in the database as 36 characters long strings.
	uuidSizeInB = 36
)

// PruneOperations deletes operations older than maxAge and larger than maxSizeMB.
// It returns the paths from objectStore that should be freed
func (st *State) PruneOperations(ctx context.Context, maxAge time.Duration, maxSizeMB int) ([]string, error) {

	// Prune by age, completed only
	ageStorePath, err := st.pruneCompletedOperationsOlderThan(ctx, maxAge)
	if err != nil {
		return nil, errors.Errorf("pruning completed operation by age: %w", err)
	}

	// Prune by size
	sizeStorePath, err := st.pruneOperationsToKeepUnderSizeMiB(ctx, maxSizeMB)
	if err != nil {
		return nil, errors.Errorf("pruning operation to keep size under the Limit: %w", err)
	}

	storePaths, err := st.deleteStoreEntryByUUIDs(ctx, append(ageStorePath, sizeStorePath...))
	if err != nil {
		return nil, errors.Errorf("deleting store entry: %w", err)
	}

	return storePaths, nil
}

// pruneCompletedOperationsOlderThan deletes operations which have completed at
// a time older than age.
func (st *State) pruneCompletedOperationsOlderThan(ctx context.Context, age time.Duration) ([]string, error) {
	db, err := st.DB(ctx)
	if err != nil {
		return nil, errors.Capture(err)
	}

	var toDeleteStorePaths []string
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		toDelete, err := st.getCompletedOperationUUIDsOlderThan(ctx, tx, age)
		if err != nil {
			return errors.Errorf("getting operation UUIDs older than %s: %w", age, err)
		}

		toDeleteStorePaths, err = st.deleteOperationByUUIDs(ctx, tx, toDelete)
		if err != nil {
			return errors.Errorf("deleting operations with UUIDs %v: %w", toDelete, err)
		}
		return nil
	})
	return toDeleteStorePaths, errors.Capture(err)
}

// getCompletedOperationUUIDsOlderThan returns the UUIDs of operations older than age.
func (st *State) getCompletedOperationUUIDsOlderThan(ctx context.Context, tx *sqlair.TX, age time.Duration) ([]string, error) {
	if age <= 0 {
		// age shouldn't be negative, but zero age is valid. In any case, we ignore
		// the prune by age as done in 3.6
		st.logger.Warningf(ctx, "Ignoring pruning by age ignored: zero age (age: %s)", age)
		return nil, nil
	}

	type expires struct {
		At time.Time `db:"at"`
	}

	type operation uuid

	expiresAt := expires{At: time.Now().Add(-age)}

	stmt, err := st.Prepare(`
	SELECT &operation.uuid 
	FROM   operation
	WHERE  completed_at IS NOT NULL
	AND    completed_at < $expires.at`,
		operation{}, expiresAt)
	if err != nil {
		return nil, errors.Capture(err)
	}

	var operations []operation
	err = tx.Query(ctx, stmt, expiresAt).GetAll(&operations)
	if err != nil && !errors.Is(err, sqlair.ErrNoRows) {
		return nil, errors.Capture(err)
	}

	return transform.Slice(operations, func(o operation) string { return o.UUID }), nil
}

// pruneOperationsToKeepUnderSizeMiB prunes operations to ensure the total size of
// operations stays below the specified Limit.
// It retrieves the database and calculates the total size and average size of
// operations. If pruning is required, it deletes a calculated number of
// operations to meet the size constraint.
// Returns the list of storeUUID to delete or an error if any issues occur during pruning.
func (st *State) pruneOperationsToKeepUnderSizeMiB(ctx context.Context, maxSizeMiB int) ([]string, error) {
	if maxSizeMiB <= 0 {
		// size shouldn't be negative, but zero size is valid. In any case, we ignore
		// the prune by size as done in 3.6
		st.logger.Warningf(ctx, "Ignoring pruning by age ignored: zero or negative size (size(MB): %s)", maxSizeMiB)
		return nil, nil
	}

	db, err := st.DB(ctx)
	if err != nil {
		return nil, errors.Capture(err)
	}

	maxSizeKiB := maxSizeMiB * humanize.KiByte
	var toDeleteStorePaths []string
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		totalSizeKiB, averageOperationSizeKiB, err := st.estimateOperationSizeInKiB(ctx, tx)
		if err != nil {
			return errors.Errorf("estimating operation size: %w", err)
		}
		if totalSizeKiB <= maxSizeKiB {
			return nil // nothing to do.
		}
		if averageOperationSizeKiB <= 0 {
			// shouldn't happen since the only reason which would happen is that
			// there is no operation in the database, so the totalSizeKiB should be
			// zero and we would have already returned nil (it would be under maxSizeKiB
			// which is strictly positive at this point
			return errors.Errorf("estimated operation size is invalid: %d", averageOperationSizeKiB)
		}

		opsToDeleteCount := (totalSizeKiB - maxSizeKiB) / averageOperationSizeKiB

		toDeleteUUIDs, err := st.getOperationToPruneUpTo(ctx, tx, opsToDeleteCount)
		if err != nil {
			return errors.Errorf("getting operation UUIDs to delete: %w", err)
		}
		toDeleteStorePaths, err = st.deleteOperationByUUIDs(ctx, tx, toDeleteUUIDs)
		return errors.Capture(err)
	})
	return toDeleteStorePaths, errors.Capture(err)
}

// estimateOperationSizeInKiB estimates the total size and average size (in KiB)
// of operations in the database.
// It calculates size based on row counts and associated object store sizes
// for operations, tasks, and logs.
// The total size is rounded up to the next multiple of humanize.KiByte.
// The average size is rounded down to the next multiple of humanize.KiByte,
// but returns at least 1.
func (st *State) estimateOperationSizeInKiB(ctx context.Context, tx *sqlair.TX) (int, int, error) {
	// Get total number of operations
	opCount, err := st.count(ctx, tx, "operation")
	if err != nil {
		return 0, -1, errors.Capture(err)
	}
	if opCount == 0 {
		return 0, -1, nil
	}

	// Table with variable content
	opSizeB, err := st.computeTableSize(ctx, tx, humanize.Byte, "operation",
		"uuid", "operation_id", "summary", "enqueued_at", "started_at", "completed_at", "parallel", "execution_group")
	if err != nil {
		return 0, -1, errors.Capture(err)
	}
	taskSizeB, err := st.computeTableSize(ctx, tx, humanize.Byte, "operation_task",
		"uuid", "operation_uuid", "task_id", "enqueued_at", "started_at", "completed_at")
	if err != nil {
		return 0, -1, errors.Capture(err)
	}
	logSizeB, err := st.computeTableSize(ctx, tx, humanize.Byte, "operation_task_log",
		"task_uuid", "content", "created_at")
	if err != nil {
		return 0, -1, errors.Capture(err)
	}
	actionSizeB, err := st.computeTableSize(ctx, tx, humanize.Byte, "operation_action",
		"operation_uuid", "charm_uuid", "charm_action_key")
	if err != nil {
		return 0, -1, errors.Capture(err)
	}
	parameterSizeB, err := st.computeTableSize(ctx, tx, humanize.Byte, "operation_parameter",
		"operation_uuid", "key", "value")
	if err != nil {
		return 0, -1, errors.Capture(err)
	}
	statusSizeB, err := st.computeTableSize(ctx, tx, humanize.Byte, "operation_task_status",
		"task_uuid", "status_id", "message", "updated_at")
	if err != nil {
		return 0, -1, errors.Capture(err)
	}

	// association table (uuid only)
	machineTaskCount, err := st.count(ctx, tx, "operation_machine_task")
	if err != nil {
		return 0, -1, errors.Capture(err)
	}
	unitTaskCount, err := st.count(ctx, tx, "operation_unit_task")
	if err != nil {
		return 0, -1, errors.Capture(err)
	}
	outputTaskCount, err := st.count(ctx, tx, "operation_task_output")
	if err != nil {
		return 0, -1, errors.Capture(err)
	}

	// Object store size
	objectStoreSizeKiB, err := st.computeObjectStoreSize(ctx, tx, humanize.Byte)
	if err != nil {
		return 0, -1, errors.Errorf("computing object store size: %w", err)
	}

	// Get total size of operation datas (in B)
	totalSizeB :=
		// precise size of operation
		opSizeB + taskSizeB + logSizeB + actionSizeB + parameterSizeB + statusSizeB +
			// association table
			machineTaskCount*2*uuidSizeInB +
			unitTaskCount*2*uuidSizeInB +
			outputTaskCount*2*uuidSizeInB +
			// object store size
			objectStoreSizeKiB

	return roundUp(totalSizeB, humanize.KiByte),
		roundDownNonZero(totalSizeB/opCount, humanize.KiByte), nil
}

// computeObjectStoreSize computes the size of the content related to tasks in
// the object store. The sizeFactor allows to convert the size in bytes to the
// size in another ratio.
func (st *State) computeObjectStoreSize(ctx context.Context, tx *sqlair.TX, sizeFactor int) (int, error) {
	type result struct {
		Size int `db:"size"`
	}
	stmt, err := st.Prepare(`
WITH
uuids AS (
    SELECT store_uuid 
    FROM operation_task_output
)
SELECT COALESCE(SUM(
    size +
    octet_length(osm.uuid) +
    octet_length(osm.sha_256) +
    octet_length(osm.sha_384) +
    octet_length(osm.size) +
    octet_length(osp.path) +
    octet_length(osp.metadata_uuid)), 0) AS &result.size
FROM   object_store_metadata AS osm
LEFT JOIN   object_store_metadata_path AS osp ON osm.uuid = osp.metadata_uuid 
WHERE  uuid IN uuids`, result{})
	if err != nil {
		return 0, errors.Capture(err)
	}
	var res result
	if err := tx.Query(ctx, stmt).Get(&res); err != nil {
		return 0, errors.Errorf("querying object store size for task content: %w", err)
	}

	return roundUp(res.Size, sizeFactor), nil
}

// roundUp rounds up the size to the next multiple of sizeFactor.
func roundUp(size int, sizeFactor int) int {
	return (size + sizeFactor - 1) / sizeFactor
}

// roundDownNonZero rounds down the size to the next multiple of sizeFactor,
// but returns at least 1.
func roundDownNonZero(size int, sizeFactor int) int {
	res := size / sizeFactor
	if res > 0 {
		return res
	}
	return 1
}

// computeTableSize computes the size of the table in the database.
// The sizeFactor allows to convert the size in bytes to the size in another ratio.
func (st *State) computeTableSize(ctx context.Context, tx *sqlair.TX, sizeFactor int, table string,
	columns ...string) (int, error) {
	type result struct {
		Size int `db:"size"`
	}
	octetLengths := transform.Slice(columns, func(col string) string {
		return fmt.Sprintf("octet_length(%q)", col)
	})
	sums := strings.Join(octetLengths, "+")
	query := fmt.Sprintf(`
SELECT COALESCE(SUM(
	%s
	), 0) AS &result.size
FROM %q
`, sums, table)
	stmt, err := st.Prepare(query, result{})
	if err != nil {
		return 0, errors.Capture(err)
	}
	var res result
	if err := tx.Query(ctx, stmt).Get(&res); err != nil {
		return 0, errors.Errorf("compute table %q size with colums %v: %w", table, columns, err)
	}
	return (res.Size + sizeFactor - 1) / sizeFactor, nil
}

// getOperationToPruneUpTo returns a list of UUIDs of operations to prune
// up to count. Operations are ordered by their completion date if any, then by
// their enqueued date (oldest first).
func (st *State) getOperationToPruneUpTo(ctx context.Context, tx *sqlair.TX, count int) ([]string, error) {
	type max struct {
		Limit int `db:"limit"`
	}
	limit := max{Limit: count}
	// Note: NULLS LAST is used to ensure that the oldest operation is deleted first.
	//  see https://sqlite.org/lang_select.html
	stmt, err := st.Prepare(`
SELECT &uuid.uuid
FROM   operation
ORDER  BY 
    completed_at ASC NULLS LAST, 
    enqueued_at ASC
LIMIT  $max.limit`, uuid{}, limit)
	if err != nil {
		return nil, errors.Capture(err)
	}
	var uuids []uuid
	if err := tx.Query(ctx, stmt, limit).GetAll(&uuids); err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, errors.Capture(err)
	}
	return transform.Slice(uuids, func(u uuid) string { return u.UUID }), nil
}
