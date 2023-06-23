// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package migration

import (
	"context"

	"github.com/juju/description/v4"
	"github.com/juju/errors"

	"github.com/juju/juju/core/database"
)

// BaseOperation is a base implementation of the Operation interface.
// The rollback operation is a no-op by default.
type BaseOperation struct{}

// Rollback is a no-op by default.
func (b *BaseOperation) Rollback(context.Context) error {
	return nil
}

// Operation is a single step in a migration.
type Operation interface {
	// Setup is called before the operation is executed. It should return an
	// error if the operation cannot be performed.
	Setup(database.DBGetter) error

	// Execute is called to perform the operation. It should return an error
	// if the operation fails.
	Execute(context.Context, description.Model) error

	// Rollback is called if the operation fails. It should attempt to undo
	// any changes made by the operation. This is best effort, and may not
	// always be possible.
	Rollback(context.Context) error
}

// Hook is a callback that is called after the operation is executed.
type Hook func(Operation) error

// Migration is a collection of operations that can be performed as a single
// unit. This is not atomic, but it does allow for a rollback of the entire
// migration if any operation fails.
type Migration struct {
	operations []Operation
	hook       Hook
}

// New creates a new migration coordinator with the given operations.
func New(operations ...Operation) *Migration {
	return &Migration{
		operations: operations,
		hook:       omitHook,
	}
}

// Add a new operation to the migration. It will be appended at the end of the
// list of operations.
func (m *Migration) Add(operations Operation) {
	m.operations = append(m.operations, operations)
}

// Len returns the number of operations in the migration.
func (m *Migration) Len() int {
	return len(m.operations)
}

// Perform executes the migration.
func (m *Migration) Perform(ctx context.Context, dbGetter database.DBGetter, model description.Model) (err error) {
	var current int
	defer func() {
		if err != nil {
			for ; current >= 0; current-- {
				if rollbackErr := m.operations[current].Rollback(ctx); rollbackErr != nil {
					err = errors.Annotatef(err, "rollback operation at %d with %v", current, rollbackErr)
				}
			}
		}
	}()

	var op Operation
	for current, op = range m.operations {
		if err := op.Setup(dbGetter); err != nil {
			return errors.Annotatef(err, "setup operation at %d", current)
		}
		if err := op.Execute(ctx, model); err != nil {
			return errors.Annotatef(err, "execute operation at %d", current)
		}
		if err := m.hook(op); err != nil {
			return errors.Annotatef(err, "hook operation at %d", current)
		}
	}
	return nil
}

// omitHook always returns a nil, omitting the error.
func omitHook(Operation) error { return nil }
