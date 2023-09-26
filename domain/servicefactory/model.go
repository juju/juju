// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package servicefactory

import (
	"github.com/juju/juju/core/changestream"
)

// ModelFactory provides access to the services required by the apiserver.
type ModelFactory struct {
	logger  Logger
	modelDB changestream.WatchableDBFactory
}

// NewModelFactory returns a new registry which uses the provided modelDB
// function to obtain a model database.
func NewModelFactory(
	modelDB changestream.WatchableDBFactory,
	logger Logger,
) *ModelFactory {
	return &ModelFactory{
		logger:  logger,
		modelDB: modelDB,
	}
}

// TODO we need a method here because if we don't have a type here, then
// anything satisfies the ModelFactory. Once we have model methods here, we
// can remove this method.
func (f *ModelFactory) TODO() {}
