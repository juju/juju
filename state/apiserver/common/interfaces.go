// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common

// Tagger is implemented by any entity with a Tag method, which should
// return the tag of the entity (for instance a machine might return
// the tag "machine-1")
type Tagger interface {
	Tag() string
}

// Authorizer represents a value that can be asked for authorization
// information on its associated authenticated entity. It is
// implemented by an API server to allow an API implementation to ask
// questions about the client that is currently connected.
type Authorizer interface {
	// AuthMachineAgent returns whether the authenticated entity is a
	// machine agent.
	AuthMachineAgent() bool

	// AuthOwner returns whether the authenticated entity is the same
	// as the given entity.
	AuthOwner(tag string) bool

	// AuthEnvironManager returns whether the authenticated entity is
	// a machine running the environment manager job.
	AuthEnvironManager() bool

	// AuthClient returns whether the authenticated entity
	// is a client user.
	AuthClient() bool
}
