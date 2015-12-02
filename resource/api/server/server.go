// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package server

import (
	"github.com/juju/loggo"
)

var logger = loggo.GetLogger("juju.resource.api.server")

// Version is the version number of the current Facade.
const Version = 1

// DataStore is the functionality of Juju's state needed for the resources API.
type DataStore interface {
	specLister
}

// Facade is the public API facade for resources.
type Facade struct {
	*specFacade
}

// NewFacade returns a new resoures facade for the given Juju state.
func NewFacade(data DataStore) *Facade {
	return &Facade{
		specFacade: &specFacade{data},
	}
}
