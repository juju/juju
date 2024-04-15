// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelmigration

import (
	"context"
	"time"

	"github.com/juju/description/v6"
	"github.com/juju/errors"

	"github.com/juju/juju/core/lease"
	"github.com/juju/juju/core/modelmigration"
	"github.com/juju/juju/domain/lease/service"
	"github.com/juju/juju/domain/lease/state"
)

const (
	// LeadershipGuarantee is the amount of time that the lease service will
	// guarantee that the application leader will be the holder of the lease.
	LeadershipGuarantee = time.Minute
)

// Coordinator is the interface that is used to add operations to a migration.
type Coordinator interface {
	// Add adds the given operation to the migration.
	Add(modelmigration.Operation)
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
	modelmigration.BaseOperation

	service ImportService
	logger  Logger
}

// Setup is called before the operation is executed. It should return an
// error if the operation cannot be performed.
func (o *importOperation) Setup(scope modelmigration.Scope) error {
	o.service = service.NewService(state.NewState(scope.ControllerDB(), o.logger))
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
		if err := o.service.ClaimLease(ctx, key, req); err != nil {
			return errors.Annotatef(err, "claiming lease for %q", key)
		}
	}

	return nil
}
