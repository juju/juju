// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelmigration

import (
	"context"

	"github.com/juju/juju/core/database"
	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/core/objectstore"
	"github.com/juju/juju/core/providertracker"
	"github.com/juju/juju/internal/errors"
)

// Operation is a single step in a migration over a payload of type P.
//
// An operation plays its part in a model migration by being instructed as part
// of a model orchestration. The coordination is required as we need to perform
// transactions over multiple databases (controller and model). This is not
// atomic, but it does allow for a rollback of the entire migration if any
// operation fails.
//
// P is the payload the operation reads from or writes to: legacy
// (description.Model) migrations instantiate Operation[description.Model], while
// the new export-format import instantiates Operation over the transformed,
// target-version payload (see domain/export/types/latest). The payload type is
// the only thing that distinguishes the two paths; everything else is shared.
type Operation[P any] interface {
	// Name returns the name of this operation.
	Name() string

	// Setup is called before the operation is executed. It should return an
	// error if the operation cannot be performed.
	Setup(Scope) error

	// Execute is called to perform the operation. It should return an error
	// if the operation fails.
	Execute(context.Context, P) error

	// Rollback is called if the operation fails. It should attempt to undo
	// any changes made by the operation. This is best effort, and may not
	// always be possible.
	// Rollback should only be called on controller DB operations. The
	// model DB operations are not rolled back, but instead we remove the
	// db, clearing the model.
	Rollback(context.Context, P) error
}

// BaseOperation is a base implementation of the [Operation] interface.
// The rollback operation is a no-op by default.
type BaseOperation[P any] struct{}

// Setup returns not implemented. It is expected that the operation will
// override this method.
func (b *BaseOperation[P]) Setup(Scope) error {
	return errors.Errorf("setup %w", coreerrors.NotImplemented)
}

// Execute returns not implemented. It is expected that the operation will
// override this method.
func (b *BaseOperation[P]) Execute(context.Context, P) error {
	return errors.Errorf("execute %w", coreerrors.NotImplemented)
}

// Rollback is a no-op by default.
func (b *BaseOperation[P]) Rollback(context.Context, P) error {
	return nil
}

// OperationAdder is the registration boundary used by per-domain RegisterImport
// functions to add their operations to a coordinator without depending on the
// concrete [Coordinator]. A *Coordinator[P] satisfies OperationAdder[P].
type OperationAdder[P any] interface {
	// Add adds the given operation to the migration.
	Add(Operation[P])
}

// Scope is a collection of resource accessors that can be used by the
// operations.
type Scope struct {
	controllerDB             database.TxnRunnerFactory
	modelDB                  database.TxnRunnerFactory
	modelObjectStoreGetter   objectstore.ModelObjectStoreGetter
	ephemeralProviderFactory providertracker.EphemeralProviderFactory
	modelUUID                model.UUID
}

// ScopeForModel returns a Scope for the given model UUID.
type ScopeForModel func(modelUUID model.UUID) Scope

// NewScope creates a new scope with the given database txn runners.
func NewScope(
	controllerDB database.TxnRunnerFactory,
	modelDB database.TxnRunnerFactory,
	modelObjectStoreGetter objectstore.ModelObjectStoreGetter,
	ephemeralProviderFactory providertracker.EphemeralProviderFactory,
	modelUUID model.UUID,
) Scope {
	return Scope{
		controllerDB:             controllerDB,
		modelDB:                  modelDB,
		modelObjectStoreGetter:   modelObjectStoreGetter,
		ephemeralProviderFactory: ephemeralProviderFactory,
		modelUUID:                modelUUID,
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

// ModelUUID returns the UUID of the model being migrated.
func (s Scope) ModelUUID() model.UUID {
	return s.modelUUID
}

// ModelObjectStoreGetter returns the object store getter for the model.
func (s Scope) ModelObjectStoreGetter() objectstore.ModelObjectStoreGetter {
	return s.modelObjectStoreGetter
}

// EphemeralProviderFactory returns the provider factory for the model.
func (s Scope) EphemeralProviderFactory() providertracker.EphemeralProviderFactory {
	return s.ephemeralProviderFactory
}

// Coordinator is a collection of operations over a payload of type P that can
// be performed as a single unit. This is not atomic, but it does allow for a
// rollback of the entire migration if any operation fails.
type Coordinator[P any] struct {
	operations []Operation[P]
	beforeEach func(context.Context) error
	logger     logger.Logger
}

// NewCoordinator creates a new migration coordinator with the given operations.
func NewCoordinator[P any](logger logger.Logger, operations ...Operation[P]) *Coordinator[P] {
	return &Coordinator[P]{
		logger:     logger,
		operations: operations,
	}
}

// Add a new operation to the migration. It will be appended at the end of the
// list of operations.
func (m *Coordinator[P]) Add(operation Operation[P]) {
	m.operations = append(m.operations, operation)
}

// Len returns the number of operations in the migration.
func (m *Coordinator[P]) Len() int {
	return len(m.operations)
}

// SetBeforeEach registers a callback invoked by Perform immediately before each
// operation runs. The new-format importer uses it to assert the durable
// model_migration_import claim is still in the importing phase before each
// model-DB write, so a concurrent abort stops the import before the next
// operation. A nil callback (the default) is a no-op.
func (m *Coordinator[P]) SetBeforeEach(beforeEach func(context.Context) error) {
	m.beforeEach = beforeEach
}

// Perform executes the migration.
// We log in addition to returning errors because the error is ultimately
// returned to the caller on the source, and we want them to be reflected
// in *this* controller's logs.
func (m *Coordinator[P]) Perform(ctx context.Context, scope Scope, payload P) (err error) {
	var current int
	defer func() {
		if err != nil {
			m.logger.Errorf(ctx, "import failed: %s", err.Error())

			for ; current >= 0; current-- {
				op := m.operations[current]

				m.logger.Infof(ctx, "rolling back operation: %s", op.Name())
				if rollbackErr := op.Rollback(ctx, payload); rollbackErr != nil {
					m.logger.Errorf(ctx, "rollback operation for %s failed: %s", op.Name(), rollbackErr)
					err = errors.Errorf("rollback operation at %d with %v: %w", current, rollbackErr, err)
				}
			}
		}
	}()

	var op Operation[P]
	for current, op = range m.operations {
		opName := op.Name()
		m.logger.Infof(ctx, "running operation: %s", opName)

		if m.beforeEach != nil {
			if err := m.beforeEach(ctx); err != nil {
				return errors.Errorf("before operation %s: %w", opName, err)
			}
		}
		if err := op.Setup(scope); err != nil {
			return errors.Errorf("setup operation %s: %w", opName, err)
		}
		if err := op.Execute(ctx, payload); err != nil {
			return errors.Errorf("execute operation %s: %w", opName, err)
		}
	}
	return nil
}
