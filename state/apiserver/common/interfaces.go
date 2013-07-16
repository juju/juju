// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common

// AuthFunc returns whether the given entity is available to some operation.
type AuthFunc func(tag string) bool

// GetAuthFunc returns an AuthFunc.
type GetAuthFunc func() (AuthFunc, error)

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

	// GetAuthTag returns the tag of the authenticated entity.
	GetAuthTag() string
}
