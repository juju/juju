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

// AuthAny returns an AuthFunc generator that returns an AuthFunc that
// accepts any tag authorized by any of its arguments. If no arguments
// are passed this is equivalent to AuthNever.
func AuthAny(getFuncs ...GetAuthFunc) GetAuthFunc {
	return func() (AuthFunc, error) {
		funcs := make([]AuthFunc, len(getFuncs))
		for i, getFunc := range getFuncs {
			f, err := getFunc()
			if err != nil {
				return nil, errors.Trace(err)
			}
			funcs[i] = f
		}
		combined := func(tag names.Tag) bool {
			for _, f := range funcs {
				if f(tag) {
					return true
				}
			}
			return false
		}
		return combined, nil
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

// AuthFuncForTag returns an authentication function that always returns true iff it is passed a specific tag.
func AuthFuncForTag(valid names.Tag) GetAuthFunc {
	return func() (AuthFunc, error) {
		return func(tag names.Tag) bool {
			return tag == valid
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
