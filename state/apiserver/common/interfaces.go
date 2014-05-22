// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common

import (
	"launchpad.net/juju-core/state"
)

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

	// AuthUnitAgent returns whether the authenticated entity is a
	// unit agent.
	AuthUnitAgent() bool

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

	// GetAuthEntity returns the authenticated entity.
	GetAuthEntity() state.Entity
}

// AuthEither returns an AuthFunc generator that returns an AuthFunc
// that accepts any tag authorized by either of its arguments.
func AuthEither(a, b GetAuthFunc) GetAuthFunc {
	return func() (AuthFunc, error) {
		f1, err := a()
		if err != nil {
			return nil, err
		}
		f2, err := b()
		if err != nil {
			return nil, err
		}
		return func(tag string) bool {
			return f1(tag) || f2(tag)
		}, nil
	}
}

// AuthAlways returns an authentication function that always returns
// the given permission.
func AuthAlways(ok bool) GetAuthFunc {
	return func() (AuthFunc, error) {
		return func(tag string) bool {
			return ok
		}, nil
	}
}
