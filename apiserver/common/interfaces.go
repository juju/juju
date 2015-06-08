// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common

import (
	"github.com/juju/errors"
	"github.com/juju/names"
)

// AuthFunc returns whether the given entity is available to some operation.
type AuthFunc func(tag names.Tag) bool

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
	AuthOwner(tag names.Tag) bool

	// AuthEnvironManager returns whether the authenticated entity is
	// a machine running the environment manager job.
	AuthEnvironManager() bool

	// AuthClient returns whether the authenticated entity
	// is a client user.
	AuthClient() bool

	// GetAuthTag returns the tag of the authenticated entity.
	GetAuthTag() names.Tag
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
		return func(tag names.Tag) bool {
			return f1(tag) || f2(tag)
		}, nil
	}
}

// AuthAlways returns an authentication function that always returns true iff it is passed a valid tag.
func AuthAlways() GetAuthFunc {
	return func() (AuthFunc, error) {
		return func(tag names.Tag) bool {
			return true
		}, nil
	}
}

// AuthNever returns an authentication function that never returns true.
func AuthNever() GetAuthFunc {
	return func() (AuthFunc, error) {
		return func(tag names.Tag) bool {
			return false
		}, nil
	}
}

// AuthFuncForTagKind returns a GetAuthFunc which creates an AuthFunc
// allowing only the given tag kind and denies all others. Passing an
// empty kind is an error.
func AuthFuncForTagKind(kind string) GetAuthFunc {
	return func() (AuthFunc, error) {
		if kind == "" {
			return nil, errors.Errorf("tag kind cannot be empty")
		}
		return func(tag names.Tag) bool {
			// Allow only the given tag kind.
			if tag == nil {
				return false
			}
			return tag.Kind() == kind
		}, nil
	}
}
