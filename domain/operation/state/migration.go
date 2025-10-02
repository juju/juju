// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"

	"github.com/canonical/sqlair"

	"github.com/juju/juju/domain/operation/internal"
	"github.com/juju/juju/internal/errors"
)

// InsertMigratingOperations inserts a new operation and its tasks.
func (st *State) InsertMigratingOperations(ctx context.Context, args internal.ImportOperationsArgs) error {
	if len(args) == 0 {
		return nil
	}

	db, err := st.DB(ctx)
	if err != nil {
		return errors.Capture(err)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		var err error

		// Insert operations
		for _, ops := range args {
			err = st.insertOperation(ctx, tx, insertOperation{
				UUID:           ops.UUID,
				OperationID:    ops.ID,
				Summary:        ops.Summary,
				EnqueuedAt:     ops.Enqueued,
				StartedAt:      nilZeroPtr(ops.Started),
				CompletedAt:    nilZeroPtr(ops.Completed),
				Parallel:       ops.IsParallel,
				ExecutionGroup: ops.ExecutionGroup,
			})
			if err != nil {
				return errors.Errorf("inserting operations at operation %q: %w", ops.ID, err)
			}

			// Insert operation parameters
			for key, value := range ops.Parameters {
				err = st.insertOperationParameter(ctx, tx, ops.UUID, key, value)
				if err != nil {
					return errors.Errorf("inserting parameter %q at operation %q: %w", key, ops.ID, err)
				}
			}

			// Insert operation action if any
			if ops.Application != "" {
				charmUUID, err := st.getCharmUUIDByApplication(ctx, tx, ops.Application)
				if err != nil {
					return errors.Errorf("getting charm UUID for application %q: %w", ops.Application, err)
				}
				err = st.insertOperationAction(ctx, tx, ops.UUID, charmUUID, ops.ActionName)
				if err != nil {
					return errors.Errorf("inserting operation action at operation %q: %w", ops.ID, err)
				}
			}

			// Insert tasks
			for _, task := range ops.Tasks {
				err = st.insertOperationTask(ctx, tx, insertOperationTask{
					UUID:          task.UUID,
					OperationUUID: ops.UUID,
					TaskID:        task.ID,
					EnqueuedAt:    task.Enqueued,
					StartedAt:     nilZeroPtr(task.Started),
					CompletedAt:   nilZeroPtr(task.Completed),
				})
				if err != nil {
					return errors.Errorf("inserting task %q at operation %q: %w", task.ID, ops.ID, err)
				}

				if task.UnitName != "" {
					err = st.insertOperationUnitTask(ctx, tx, task.UUID, task.UnitName)
					if err != nil {
						return errors.Errorf("inserting task %q unit receiver %q at operation %q: %w",
							task.ID, task.UnitName, ops.ID, err)
					}
				}
				if task.MachineName != "" {
					machineUUID, err := st.getMachineUUID(ctx, tx, task.MachineName)
					if err != nil {
						return errors.Errorf("getting machine UUID for %q: %w", task.MachineName, err)
					}
					err = st.insertOperationMachineTask(ctx, tx, task.UUID, machineUUID)
					if err != nil {
						return errors.Errorf("inserting task %q machine receiver %q at operation %q: %w",
							task.ID, task.MachineName, ops.ID, err)
					}
				}

				err = st.insertOperationTaskOutputIfAny(ctx, tx, task.UUID, task.StorePath)
				if err != nil {
					return errors.Errorf("inserting task %q output store %q at operation %q: %w",
						task.ID, task.StorePath, ops.ID, err)
				}

				err = st.insertOperationTaskStatus(ctx, tx, task.UUID, task.Status)
				if err != nil {
					return errors.Errorf("inserting task %q status at operation %q: %w", task.ID, ops.ID, err)
				}
				for _, log := range task.Log {
					err = st.insertTaskMessage(ctx, tx, task.ID, log.Timestamp, log.Message)
					if err != nil {
						return errors.Errorf("inserting task %q log at operation %q: %w", task.ID, ops.ID, err)
					}
				}
			}
		}

		return errors.Capture(err)
	})
	if err != nil {
		return errors.Errorf("adding exec operation: %w", err)
	}

	return nil
}

// DeleteImportedOperations deletes all imported operations in a model during rollback.
// it returns all the storePaths of the deleted operations.
func (st *State) DeleteImportedOperations(ctx context.Context) ([]string, error) {
	db, err := st.DB(ctx)
	if err != nil {
		return nil, errors.Capture(err)
	}

	var storePaths []string
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		opUUIDs, err := st.getAllOperationUUIDs(ctx, tx)
		if err != nil {
			return errors.Errorf("getting all operation UUIDs: %w", err)
		}
		storePaths, err = st.deleteOperationByUUIDs(ctx, tx, opUUIDs)
		if err != nil {
			return errors.Errorf("deleting operations: %w", err)
		}

		return nil
	})
	if err != nil {
		return nil, errors.Errorf("deleting imported operations: %w", err)
	}

	return storePaths, nil
}

// nilZeroPtr returns a pointer to the given value if it is not zero, or nil if it is.
func nilZeroPtr[T comparable](completed T) *T {
	var zero T
	if completed == zero {
		return nil
	}
	return &completed
}
