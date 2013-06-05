// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common

// Tagger interface defines a single Tag method.
type Tagger interface {
	Tag() string
}

// Authorizer interface defines per-method authorization calls.
type Authorizer interface {
	AuthOwner(entity Tagger) bool
	AuthEnvironManager() bool
}
