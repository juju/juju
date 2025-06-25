// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package facadetest

import (
	"context"

	"github.com/juju/juju/core/model"
	"github.com/juju/juju/core/objectstore"
	"github.com/juju/juju/internal/services"
	"github.com/juju/juju/internal/testing"
)

// MultiModelContext implements facade.MultiModelContext in the simplest
// possible way.
type MultiModelContext struct {
	ModelContext

	DomainServicesForModelFunc_ func(model.UUID) services.DomainServices
	DomainServicesForModel_     services.DomainServices
	ObjectStoreForModel_        objectstore.ObjectStore
}

// DomainServicesForModel returns the services factory for a given model uuid.
func (c MultiModelContext) DomainServicesForModel(ctx context.Context, uuid model.UUID) (services.DomainServices, error) {
	if c.DomainServicesForModelFunc_ != nil {
		return c.DomainServicesForModelFunc_(uuid), nil
	}
	return c.DomainServicesForModel_, nil
}

// ObjectStoreForModel returns the object store for a given model uuid.
func (c MultiModelContext) ObjectStoreForModel(ctx context.Context, modelUUID string) (objectstore.ObjectStore, error) {
	return c.ObjectStoreForModel_, nil
}

// ControllerModelUUID returns the UUID of the controller model.
func (c MultiModelContext) ControllerModelUUID() model.UUID {
	return model.UUID(testing.ControllerModelTag.Id())
}
