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
	// IsLoggedIn returns whether the user is currently logged in and
	// authenticated.
	IsLoggedIn() bool

	// AuthMachineAgent returns whether the authenticated entity is a
	// machine agent.
	AuthMachineAgent() bool

	// AuthOwner returns whether the authenticated entity is the same
	// as the given entity.
	AuthOwner(tag string) bool

	// AuthEnvironManager returns whether the authenticated entity is
	// a machine running the environment manager job.
	AuthEnvironManager() bool
}

// Resource represents any resource that should be cleaned up when an
// API connection terminates. The Stop method will be called when
// that happens.
type Resource interface {
	Stop() error
}

// ResourceRegistry is an interface that allows the registration of
// resources that will be cleaned up when an API connection
// terminates. It is typically implemented by an API server.
type ResourceRegistry interface {
	// RegisterResource registers the given resource. It returns a unique
	// identifier for the resource which can then be used in
	// subsequent API requests to refer to the resource.
	Register(resource Resource) string

	// GetResource returns the resource for the given id, or
	// nil if there is no such resource.
	Get(id string) Resource

	// StopResource stops the resource with the given id and unregisters it.
	// It returns any error from the underlying Stop call.
	// It does not return an error if the resource has already
	// been unregistered.
	Stop(id string) error
}
