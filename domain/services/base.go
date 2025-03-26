// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package services

import (
	"github.com/juju/juju/core/changestream"
	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/domain"
)

// serviceFactoryBase is the foundation for all service factories.
// It encapsulates the ability to supply transaction runners and watchers
// backed by the controller database.
type serviceFactoryBase struct {
	controllerDB changestream.WatchableDBFactory
	logger       logger.Logger
}

func (s *serviceFactoryBase) controllerWatcherFactory(logger logger.Logger) *domain.WatcherFactory {
	return domain.NewWatcherFactory(
		s.controllerDB,
		logger,
	)
}

// loggerFor returns a logger with the given name as a child of the factory's
// logger.
func (s *serviceFactoryBase) loggerFor(name string) logger.Logger {
	return s.logger.Child(name)
}

// modelServiceFactoryBase is the foundation for model-scoped service factories.
// It includes the ability to supply runners and watchers backed by a model
// database in addition to those backed by the controller database.
type modelServiceFactoryBase struct {
	serviceFactoryBase

	modelDB changestream.WatchableDBFactory
}

func (s *modelServiceFactoryBase) modelWatcherFactory(logger logger.Logger) *domain.WatcherFactory {
	return domain.NewWatcherFactory(
		s.modelDB,
		logger,
	)
}
