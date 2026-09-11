// Copyright 2026 Canonical Ltd. All rights reserved.
// Licensed under the AGPLv3, see LICENCE file for details.

package model

import (
	"context"
	"database/sql"
	"testing"

	"github.com/juju/tc"

	"github.com/juju/juju/domain/life"
	schematesting "github.com/juju/juju/domain/schema/testing"
)

type nullableExportSuite struct {
	schematesting.ModelSuite
}

func TestNullableExportSuite(t *testing.T) {
	tc.Run(t, &nullableExportSuite{})
}

// TestExportStorageFilesystemPreservesNullableValues verifies that the
// companion null marker retains the original nullness of the Boolean
// obliterate_on_cleanup value.
func (s *nullableExportSuite) TestExportStorageFilesystemPreservesNullableValues(c *tc.C) {
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `
INSERT INTO storage_filesystem
    (uuid, filesystem_id, life_id, provision_scope_id, obliterate_on_cleanup)
VALUES
    ('fs-null', 'fs-null', ?, 0, NULL),
    ('fs-false', 'fs-false', ?, 0, FALSE),
    ('fs-true', 'fs-true', ?, 0, TRUE)
`, life.Alive, life.Dying, life.Dead)
		return err
	})
	c.Assert(err, tc.ErrorIsNil)

	payload, err := NewState(s.TxnRunnerFactory()).Export(c.Context())
	c.Assert(err, tc.ErrorIsNil)

	filesystems := make(map[string]*bool)
	for _, filesystem := range payload.StorageFilesystem {
		filesystems[filesystem.UUID] = filesystem.ObliterateOnCleanup
	}
	c.Check(filesystems["fs-null"], tc.IsNil)
	c.Assert(filesystems["fs-false"], tc.NotNil)
	c.Check(*filesystems["fs-false"], tc.IsFalse)
	c.Assert(filesystems["fs-true"], tc.NotNil)
	c.Check(*filesystems["fs-true"], tc.IsTrue)
}

// TestExportStorageFilesystemStatusPreservesNullableValues verifies that the
// companion null marker retains the original nullness of the datetime
// updated_at value.
func (s *nullableExportSuite) TestExportStorageFilesystemStatusPreservesNullableValues(c *tc.C) {
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, `
INSERT INTO storage_filesystem
    (uuid, filesystem_id, life_id, provision_scope_id)
VALUES
    ('fs-null', 'fs-null', ?, 0),
    ('fs-true', 'fs-true', ?, 0)
`, life.Alive, life.Alive); err != nil {
			return err
		}
		_, err := tx.ExecContext(ctx, `
INSERT INTO storage_filesystem_status
    (filesystem_uuid, status_id, updated_at)
VALUES
    ('fs-null', 0, NULL),
    ('fs-true', 0, '2026-09-10 12:34:56')
`)
		return err
	})
	c.Assert(err, tc.ErrorIsNil)

	payload, err := NewState(s.TxnRunnerFactory()).Export(c.Context())
	c.Assert(err, tc.ErrorIsNil)

	filesystemStatuses := make(map[string]bool)
	for _, status := range payload.StorageFilesystemStatus {
		filesystemStatuses[status.FilesystemUUID] = status.UpdatedAt == nil
	}
	c.Check(filesystemStatuses["fs-null"], tc.IsTrue)
	c.Check(filesystemStatuses["fs-true"], tc.IsFalse)
}

// TestExportOperationPreservesNullableValues verifies that the companion null
// markers retain the original nullness of the datetime started_at and
// completed_at values and the Boolean parallel value.
func (s *nullableExportSuite) TestExportOperationPreservesNullableValues(c *tc.C) {
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `
INSERT INTO operation
    (uuid, operation_id, enqueued_at, started_at, completed_at, parallel)
VALUES
    ('op-null', 'op-null', '2026-09-10 12:34:56', NULL, NULL, NULL),
    ('op-value', 'op-value', '2026-09-10 12:34:56',
     '2026-09-10 12:35:56', '2026-09-10 12:36:56', TRUE)
`)
		return err
	})
	c.Assert(err, tc.ErrorIsNil)

	payload, err := NewState(s.TxnRunnerFactory()).Export(c.Context())
	c.Assert(err, tc.ErrorIsNil)

	operations := make(map[string]struct {
		startedAt   bool
		completedAt bool
		parallel    *bool
	})
	for _, operation := range payload.Operation {
		operations[operation.UUID] = struct {
			startedAt   bool
			completedAt bool
			parallel    *bool
		}{
			startedAt:   operation.StartedAt == nil,
			completedAt: operation.CompletedAt == nil,
			parallel:    operation.Parallel,
		}
	}
	c.Check(operations["op-null"].startedAt, tc.IsTrue)
	c.Check(operations["op-null"].completedAt, tc.IsTrue)
	c.Check(operations["op-null"].parallel, tc.IsNil)
	c.Check(operations["op-value"].startedAt, tc.IsFalse)
	c.Check(operations["op-value"].completedAt, tc.IsFalse)
	c.Assert(operations["op-value"].parallel, tc.NotNil)
	c.Check(*operations["op-value"].parallel, tc.IsTrue)
}

// TestExportOperationTaskPreservesNullableValues verifies that the companion
// null markers retain the original nullness of the datetime started_at and
// completed_at values.
func (s *nullableExportSuite) TestExportOperationTaskPreservesNullableValues(c *tc.C) {
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, `
INSERT INTO operation
    (uuid, operation_id, enqueued_at)
VALUES
    ('op-null', 'op-null', '2026-09-10 12:34:56'),
    ('op-value', 'op-value', '2026-09-10 12:34:56')
`); err != nil {
			return err
		}
		_, err := tx.ExecContext(ctx, `
INSERT INTO operation_task
    (uuid, operation_uuid, task_id, enqueued_at, started_at, completed_at)
VALUES
    ('task-null', 'op-null', 'task-null', '2026-09-10 12:34:56', NULL, NULL),
    ('task-value', 'op-value', 'task-value', '2026-09-10 12:34:56',
     '2026-09-10 12:35:56', '2026-09-10 12:36:56')
`)
		return err
	})
	c.Assert(err, tc.ErrorIsNil)

	payload, err := NewState(s.TxnRunnerFactory()).Export(c.Context())
	c.Assert(err, tc.ErrorIsNil)

	operationTasks := make(map[string]struct {
		startedAt   bool
		completedAt bool
	})
	for _, task := range payload.OperationTask {
		operationTasks[task.UUID] = struct {
			startedAt   bool
			completedAt bool
		}{
			startedAt:   task.StartedAt == nil,
			completedAt: task.CompletedAt == nil,
		}
	}
	c.Check(operationTasks["task-null"].startedAt, tc.IsTrue)
	c.Check(operationTasks["task-null"].completedAt, tc.IsTrue)
	c.Check(operationTasks["task-value"].startedAt, tc.IsFalse)
	c.Check(operationTasks["task-value"].completedAt, tc.IsFalse)
}

// TestExportStorageVolumePreservesNullableValues verifies that the companion
// null marker retains the original nullness of the Boolean
// obliterate_on_cleanup value.
func (s *nullableExportSuite) TestExportStorageVolumePreservesNullableValues(c *tc.C) {
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `
INSERT INTO storage_volume
    (uuid, volume_id, life_id, provision_scope_id, obliterate_on_cleanup)
VALUES
    ('vol-null', 'vol-null', ?, 0, NULL),
    ('vol-false', 'vol-false', ?, 0, FALSE),
    ('vol-true', 'vol-true', ?, 0, TRUE)
`, life.Alive, life.Dying, life.Dead)
		return err
	})
	c.Assert(err, tc.ErrorIsNil)

	payload, err := NewState(s.TxnRunnerFactory()).Export(c.Context())
	c.Assert(err, tc.ErrorIsNil)

	volumes := make(map[string]*bool)
	for _, volume := range payload.StorageVolume {
		volumes[volume.UUID] = volume.ObliterateOnCleanup
	}
	c.Check(volumes["vol-null"], tc.IsNil)
	c.Assert(volumes["vol-false"], tc.NotNil)
	c.Check(*volumes["vol-false"], tc.IsFalse)
	c.Assert(volumes["vol-true"], tc.NotNil)
	c.Check(*volumes["vol-true"], tc.IsTrue)
}
