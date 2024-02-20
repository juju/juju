// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package facadetest

import (
	"context"

	"github.com/juju/juju/core/objectstore"
	"github.com/juju/juju/internal/servicefactory"
)

// ModelContext implements facade.ModelContext in the simplest possible way.
type ModelContext struct {
	Context

	ServiceFactoryForModel_ servicefactory.ServiceFactory
	ObjectStoreForModel_    objectstore.ObjectStore
}

// ServiceFactoryForModel returns the services factory for a given
// model uuid.
func (c ModelContext) ServiceFactoryForModel(modelUUID string) servicefactory.ServiceFactory {
	return c.ServiceFactoryForModel_
}

// ObjectStoreForModel returns the object store for a given model uuid.
func (c ModelContext) ObjectStoreForModel(ctx context.Context, modelUUID string) (objectstore.ObjectStore, error) {
	return c.ObjectStoreForModel_, nil
}
