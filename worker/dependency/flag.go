// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package dependency

import (
	"github.com/juju/errors"

	"github.com/juju/juju/worker"
)

// Flag represents a single boolean used to determine whether a given
// manifold worker should run.
type Flag interface {

	// Check returns the flag's value. Check calls must *always* return
	// the same value for a given instatiation of the type implementing
	// Flag.
	Check() bool
}

// WithFlag returns a manifold, based on that supplied, which will only run
// a worker when the named flag manifold's worker is active and set.
func WithFlag(base Manifold, flagName string) Manifold {
	return Manifold{
		Inputs: append(base.Inputs, flagName),
		Start:  flagWrap(base.Start, flagName),
		Output: base.Output,
	}
}

// flagWrap returns a StartFunc that will return ErrMissing if the named flag
// resource is not active or not set.
func flagWrap(inner StartFunc, flagName string) StartFunc {
	return func(getResource GetResourceFunc) (worker.Worker, error) {
		var flag Flag
		if err := getResource(flagName, &flag); err != nil {
			return nil, errors.Trace(err)
		}
		if !flag.Check() {
			return nil, ErrMissing
		}

		worker, err := inner(getResource)
		if err != nil {
			return nil, errors.Trace(err)
		}
		return worker, nil
	}
}
