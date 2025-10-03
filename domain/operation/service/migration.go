// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"

	"github.com/juju/collections/transform"

	"github.com/juju/juju/core/trace"
	"github.com/juju/juju/domain/operation/internal"
	"github.com/juju/juju/internal/errors"
	"github.com/juju/juju/internal/uuid"
)

// InsertMigratingOperations sets operations imported in migration.
func (s *Service) InsertMigratingOperations(ctx context.Context, args internal.ImportOperationsArgs) error {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	args = transform.Slice(args, func(arg internal.ImportOperationArg) internal.ImportOperationArg {
		if arg.UUID == "" {
			arg.UUID = uuid.MustNewUUID().String()
		}
		return arg
	})

	var err error
	var rollbacks []func()
	defer func() {
		if err != nil {
			for _, rollback := range rollbacks {
				rollback()
			}
		}
	}()

	for opIdx := range args {
		op := &args[opIdx]
		// first, generate the UUIDs if required for the operations
		if op.UUID == "" {
			op.UUID = uuid.MustNewUUID().String()
		}

		// then update the task with UUID and store the results
		for taskIdx := range op.Tasks {
			task := &args[opIdx].Tasks[taskIdx]
			if task.UUID == "" {
				task.UUID = uuid.MustNewUUID().String()
			}

			if len(task.Output) == 0 {
				continue
			}

			var storePath string
			var rollback func()
			// warning: err should be setted, not defined to properly handle rollbacks
			storePath, rollback, err = s.storeTaskResults(ctx, task.UUID, task.Output)
			if err != nil {
				return errors.Errorf("putting task result %q in store: %w", task.UUID, err)
			}
			rollbacks = append(rollbacks, rollback)
			task.StorePath = storePath
		}
	}
	// warning: err should be setted, not defined to properly handle rollbacks
	err = s.st.InsertMigratingOperations(ctx, args)
	return errors.Capture(err)
}

// DeleteImportedOperations deletes all imported operations in a model during
// an import rollback.
func (s *Service) DeleteImportedOperations(ctx context.Context) error {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	storePaths, err := s.st.DeleteImportedOperations(ctx)
	if err != nil {
		return errors.Errorf("deleting imported operations: %w", err)
	}
	if len(storePaths) == 0 {
		return nil
	}

	store, err := s.objectStoreGetter.GetObjectStore(ctx)
	if err != nil {
		return errors.Errorf("getting object store: %w", err)
	}
	for _, path := range storePaths {
		err := store.Remove(ctx, path)
		if err != nil {
			s.logger.Warningf(ctx, "error deleting object store entry while rollback migration %q: %v", path, err)
		}
	}

	return nil
}
