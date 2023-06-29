// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package migrations

import (
	"context"
	"time"

	"github.com/juju/description/v4"

	"github.com/juju/juju/core/lease"
	"github.com/juju/juju/core/migration"
	"github.com/juju/juju/domain"
	"github.com/juju/juju/domain/lease/service"
	"github.com/juju/juju/domain/lease/state"
)

const (
	// LeadershipGuarantee is the amount of time that the lease service will
	// guarantee that the application leader will be the holder of the lease.
	LeadershipGuarantee = time.Second * 30
)

// Coordinator is the interface that is used to add operations to a migration.
type Coordinator interface {
	// Add adds the given operation to the migration.
	Add(migration.Operation)
}

// Logger is the interface that is used to log messages.
type Logger interface {
	Infof(string, ...any)
	Debugf(string, ...any)
}

// RegisterImport registers the import operations with the given coordinator.
func RegisterImport(coordinator Coordinator, logger Logger) {
	coordinator.Add(&importOperation{
		logger: logger,
	})
}

// ImportService is the interface that is used by the import operations to
// interact with the lease service.
type ImportService interface {
	ClaimLease(context.Context, lease.Key, lease.Request) error
}

type importOperation struct {
	migration.BaseOperation

	service ImportService
	logger  Logger
}

// Setup is called before the operation is executed. It should return an
// error if the operation cannot be performed.
func (o *importOperation) Setup(scope migration.Scope) error {
	o.service = service.NewService(state.NewState(domain.ConstFactory(scope.ControllerDB()), o.logger))
	return nil
}

// Execute is called to perform the operation. It should return an error
// if the operation fails.
func (o *importOperation) Execute(ctx context.Context, model description.Model) error {
	for _, app := range model.Applications() {
		key := lease.Key{
			ModelUUID: model.Tag().Id(),
			Namespace: app.Name(),
			Lease:     lease.ApplicationLeadershipNamespace,
		}
		req := lease.Request{
			Holder:   app.Leader(),
			Duration: LeadershipGuarantee,
		}
		o.service.ClaimLease(ctx, key, req)
	}

	return nil
}
