// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package services

import (
	"github.com/juju/clock"

	"github.com/juju/juju/core/changestream"
	"github.com/juju/juju/core/database"
	"github.com/juju/juju/core/lease"
	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/core/objectstore"
	"github.com/juju/juju/core/providertracker"
	"github.com/juju/juju/core/storage"
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
	objectStoreGetter objectstore.ModelObjectStoreGetter,
	storageRegistry storage.ModelStorageRegistryGetter,
	publicKeyImporter PublicKeyImporter,
	leaseManager lease.ModelLeaseManagerGetter,
	clock clock.Clock,
	logger logger.Logger,
) *DomainServices {
	controllerServices := NewControllerServices(controllerDB, deleterDB, clock, logger)
	return &DomainServices{
		ControllerServices: controllerServices,
		ModelFactory: NewModelFactory(
			modelUUID,
			controllerDB,
			modelDB,
			providerTracker,
			objectStoreGetter,
			storageRegistry,
			publicKeyImporter,
			leaseManager,
			clock,
			logger,
		),
	}
}
