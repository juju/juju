// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package servicefactory

import (
	"github.com/juju/juju/core/changestream"
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
	logger Logger,
) *ModelFactory {
	return &ModelFactory{
		modelDB: modelDB,
		logger:  logger,
	}
}

func (f *ModelFactory) Name() string {
	return "ModelFactory"
}
