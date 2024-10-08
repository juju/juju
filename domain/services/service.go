// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package services

import (
	"github.com/juju/clock"

	"github.com/juju/juju/core/changestream"
	"github.com/juju/juju/core/database"
	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/core/objectstore"
	"github.com/juju/juju/core/providertracker"
)

// DomainServices provides access to the services required by the apiserver.
type DomainServices struct {
	*ControllerServices
	*ModelFactory
}

// NewDomainServices returns a new domain services which can be used to
// get new services from.
func NewDomainServices(
	controllerDB changestream.WatchableDBFactory,
	modelUUID model.UUID,
	modelDB changestream.WatchableDBFactory,
	deleterDB database.DBDeleter,
	providerTracker providertracker.ProviderFactory,
	objectStore objectstore.ModelObjectStoreGetter,
	logger logger.Logger,
	clock clock.Clock,
) *DomainServices {
	controllerServices := NewControllerServices(controllerDB, deleterDB, logger)
	return &DomainServices{
		ControllerServices: controllerServices,
		ModelFactory: NewModelFactory(
			modelUUID,
			controllerDB,
			modelDB,
			providerTracker,
			objectStore,
			logger,
			clock,
		),
	}
}
