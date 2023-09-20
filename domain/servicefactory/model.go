// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package servicefactory

import (
	"github.com/juju/juju/core/changestream"
	"github.com/juju/juju/domain/model"
)

// ModelFactory provides access to the services required by the apiserver.
type ModelFactory struct {
	logger  Logger
	modelDB changestream.WatchableModelDBFactory
}

// NewModelFactory returns a new registry which uses the provided modelDB
// function to obtain a model database.
func NewModelFactory(
	modelDB changestream.WatchableModelDBFactory,
	logger Logger,
) *ModelFactory {
	return &ModelFactory{
		logger:  logger,
		modelDB: modelDB,
	}
}

// ModelUUID is the current UUID for the model.
func (f *ModelFactory) ModelUUID() model.UUID {
	return f.modelDB.ModelUUID
}
