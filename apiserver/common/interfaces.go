// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common

import (
	"github.com/juju/errors"
	"gopkg.in/juju/names.v2"
)

// AuthFunc returns whether the given entity is available to some operation.
type AuthFunc func(tag names.Tag) bool

// GetAuthFunc returns an AuthFunc.
type GetAuthFunc func() (AuthFunc, error)

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
