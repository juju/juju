// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package servicefactory

import (
	"github.com/juju/juju/core/changestream"
	"github.com/juju/juju/core/database"
)

// ModelFactory provides access to the services required by the apiserver.
type ModelFactory struct {
	modelDB changestream.WatchableDBFactory
	logger  Logger
}

// NewModelFactory returns a new registry which uses the provided modelDB
// function to obtain a model database.
func NewModelFactory(
	modelDB changestream.WatchableDBFactory,
	deleterDB database.DBDeleter,
	logger Logger,
) *ModelFactory {
	return &ModelFactory{
		modelDB: modelDB,
		logger:  logger,
	}
}
