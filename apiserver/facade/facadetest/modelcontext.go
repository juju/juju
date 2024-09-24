// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package facadetest

import (
	"context"

	"github.com/juju/juju/core/model"
	"github.com/juju/juju/core/objectstore"
	"github.com/juju/juju/internal/services"
)

// MultiModelContext implements facade.MultiModelContext in the simplest
// possible way.
type MultiModelContext struct {
	ModelContext

	DomainServicesForModel_ services.DomainServices
	ObjectStoreForModel_    objectstore.ObjectStore
}

// DomainServicesForModel returns the services factory for a given model uuid.
func (c MultiModelContext) DomainServicesForModel(uuid model.UUID) services.DomainServices {
	return c.DomainServicesForModel_
}

// ObjectStoreForModel returns the object store for a given model uuid.
func (c MultiModelContext) ObjectStoreForModel(ctx context.Context, modelUUID string) (objectstore.ObjectStore, error) {
	return c.ObjectStoreForModel_, nil
}
