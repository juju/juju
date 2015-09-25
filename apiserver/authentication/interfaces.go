// Copyright 2014 Canonical Ltd. All rights reserved.
// Licensed under the AGPLv3, see LICENCE file for details.

package authentication

import (
	"github.com/juju/names"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/state"
)

// EntityAuthenticator is the interface all entity authenticators need to implement
// to authenticate juju entities.
type EntityAuthenticator interface {
	// Authenticate authenticates the given entity
	Authenticate(entityFinder EntityFinder, tag names.Tag, req params.LoginRequest) (state.Entity, error)
}

// EntityFinder finds the entity described by the tag.
type EntityFinder interface {
	FindEntity(tag names.Tag) (state.Entity, error)
}
