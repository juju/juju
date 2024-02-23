// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package servicefactory

import (
	"github.com/juju/juju/core/changestream"
	"github.com/juju/juju/core/database"
)

// ServiceFactory provides access to the services required by the apiserver.
type ServiceFactory struct {
	*ControllerFactory
	*ModelFactory
}

// NewServiceFactory returns a new service factory which can be used to
// get new services from.
func NewServiceFactory(
	controllerDB changestream.WatchableDBFactory,
	modelDB changestream.WatchableDBFactory,
	deleterDB database.DBDeleter,
	environFactory EnvironFactory,
	logger Logger,
) *ServiceFactory {
	controllerFactory := NewControllerFactory(controllerDB, deleterDB, logger)
	return &ServiceFactory{
		ControllerFactory: controllerFactory,
		ModelFactory: NewModelFactory(
			modelDB,
			environFactory,
			logger,
		),
	}
}
