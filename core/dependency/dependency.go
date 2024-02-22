// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package dependency

import (
	"github.com/juju/errors"
	"github.com/juju/worker/v4/dependency"
)

// GetDependencyByName is a helper function that gets a dependency by name and
// calls a function with the dependency as an argument.
// The functor (A -> B) is applied to the dependency to either get a sub
// dependency or to return the dependency itself.
func GetDependencyByName[A, B any](getter dependency.Getter, name string, fn func(A) (B, error)) (B, error) {
	var dependency A
	if err := getter.Get(name, &dependency); err != nil {
		var b B
		return b, errors.Trace(err)
	}

	return fn(dependency)
}

// Identity is a helper function that returns the argument as the same type.
// This will panic if A and B aren't compatible types.
func Identity[A, B any](a A) (B, error) {
	b, ok := any(a).(B)
	if !ok {
		return b, errors.Errorf("cannot convert %T to %T", a, b)
	}
	return b, nil
}
