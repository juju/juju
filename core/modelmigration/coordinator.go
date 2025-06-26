// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelmigration

import (
	"context"

	"github.com/juju/description/v10"

	"github.com/juju/juju/core/database"
	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/internal/errors"
)

// BaseOperation is a base implementation of the Operation interface.
// The rollback operation is a no-op by default.
type BaseOperation struct{}

// Setup returns not implemented. It is expected that the operation will
// override this method.
func (b *BaseOperation) Setup(Scope) error {
	return errors.Errorf("setup %w", coreerrors.NotImplemented)
}

// Execute returns not implemented. It is expected that the operation will
// override this method.
func (b *BaseOperation) Execute(context.Context, description.Model) error {
	return errors.Errorf("execute %w", coreerrors.NotImplemented)
}

// Rollback is a no-op by default.
func (b *BaseOperation) Rollback(context.Context, description.Model) error {
	return nil
}

// Operation is a single step in a migration.
// An operation plays its part in the model migration by being instructed as
// part of a model orchestration. The coordination is required as we need to
// perform transactions over multiple databases (controller and model). This
// is not atomic, but it does allow for a rollback of the entire migration if
// any operation fails.
type Operation interface {
	// Name returns the name of this operation.
	Name() string

	// Setup is called before the operation is executed. It should return an
	// error if the operation cannot be performed.
	Setup(Scope) error

	// Execute is called to perform the operation. It should return an error
	// if the operation fails.
	Execute(context.Context, description.Model) error

	// Rollback is called if the operation fails. It should attempt to undo
	// any changes made by the operation. This is best effort, and may not
	// always be possible.
	// Rollback should only be called on controller DB operations. The
	// model DB operations are not rolled back, but instead we remove the
	// db, clearing the model.
	Rollback(context.Context, description.Model) error
}

// Scope is a collection of database txn runners that can be used by the
// operations.
type Scope struct {
	controllerDB database.TxnRunnerFactory
	modelDB      database.TxnRunnerFactory
	modelDeleter database.DBDeleter
}

// ScopeForModel returns a Scope for the given model UUID.
type ScopeForModel func(modelUUID model.UUID) Scope

// NewScope creates a new scope with the given database txn runners.
func NewScope(controllerDB, modelDB database.TxnRunnerFactory, modelDeleter database.DBDeleter) Scope {
	return Scope{
		controllerDB: controllerDB,
		modelDB:      modelDB,
		modelDeleter: modelDeleter,
	}
}

// ControllerDB returns the database txn runner for the controller.
func (s Scope) ControllerDB() database.TxnRunnerFactory {
	return s.controllerDB
}

// ModelDB returns the database txn runner for the model.
func (s Scope) ModelDB() database.TxnRunnerFactory {
	return s.modelDB
}

// ModelDeleter returns the database deleter for the model.
func (s Scope) ModelDeleter() database.DBDeleter {
	return s.modelDeleter
}

// Hook is a callback that is called after the operation is executed.
type Hook func(Operation) error

// Coordinator is a collection of operations that can be performed as a single
// unit. This is not atomic, but it does allow for a rollback of the entire
// migration if any operation fails.
type Coordinator struct {
	operations []Operation
	hook       Hook
	logger     logger.Logger
}

// NewCoordinator creates a new migration coordinator with the given operations.
func NewCoordinator(logger logger.Logger, operations ...Operation) *Coordinator {
	return &Coordinator{
		logger:     logger,
		operations: operations,
		hook:       emptyHook,
	}
}

// Add a new operation to the migration. It will be appended at the end of the
// list of operations.
func (m *Coordinator) Add(operations Operation) {
	m.operations = append(m.operations, operations)
}

// Len returns the number of operations in the migration.
func (m *Coordinator) Len() int {
	return len(m.operations)
}

// Perform executes the migration.
// We log in addition to returning errors because the error is ultimately
// returned to the caller on the source, and we want them to be reflected
// in *this* controller's logs.
func (m *Coordinator) Perform(ctx context.Context, scope Scope, model description.Model) (err error) {
	var current int
	defer func() {
		if err != nil {
			m.logger.Errorf(context.TODO(), "import failed: %s", err.Error())

			for ; current >= 0; current-- {
				op := m.operations[current]

				m.logger.Infof(context.TODO(), "rolling back operation: %s", op.Name())
				if rollbackErr := op.Rollback(ctx, model); rollbackErr != nil {
					m.logger.Errorf(context.TODO(), "rollback operation for %s failed: %s", op.Name(), rollbackErr)
					err = errors.Errorf("rollback operation at %d with %v: %w", current, rollbackErr, err)
				}
			}
		}
	}()

	var op Operation
	for current, op = range m.operations {
		opName := op.Name()
		m.logger.Infof(context.TODO(), "running operation: %s", opName)

		if err := op.Setup(scope); err != nil {
			return errors.Errorf("setup operation %s: %w", opName, err)
		}
		if err := op.Execute(ctx, model); err != nil {
			return errors.Errorf("execute operation %s: %w", opName, err)
		}
		if err := m.hook(op); err != nil {
			return errors.Errorf("hook operation %s: %w", opName, err)
		}
	}
	return nil
}

// emptyHook always returns a nil, omitting the error.
func emptyHook(Operation) error { return nil }
