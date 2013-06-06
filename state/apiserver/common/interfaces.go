// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common

// Tagger is implemented by any entity with a Tag method, which should
// return the tag of the entity (for instance a machine might return
// the tag "machine-1")
type Tagger interface {
	Tag() string
}

// Authorizer interface defines per-method authorization calls.
type Authorizer interface {
	AuthOwner(entity Tagger) bool
	AuthEnvironManager() bool
}
