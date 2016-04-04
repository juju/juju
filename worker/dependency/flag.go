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

// FlagOutput will expose, as a Flag, any worker that implements Flag.
func FlagOutput(in worker.Worker, out interface{}) error {
	inFlag, ok := in.(Flag)
	if !ok {
		return errors.Errorf("expected in to implement Flag; got a %T", in)
	}
	outFlag, ok := out.(*Flag)
	if !ok {
		return errors.Errorf("expected out to be a *Flag; got a %T", out)
	}
	*outFlag = inFlag
	return nil
}

// WithFlag returns a manifold, based on that supplied, which will only run
// a worker when the named flag manifold's worker is active and set.
func WithFlag(base Manifold, flagName string) Manifold {
	return Manifold{
		Inputs: append(base.Inputs, flagName),
		Start:  flagWrap(base.Start, flagName),
		Output: base.Output,
		Filter: base.Filter,
	}
}

// flagWrap returns a StartFunc that will return ErrMissing if the named flag
// resource is not active or not set.
func flagWrap(inner StartFunc, flagName string) StartFunc {
	return func(context Context) (worker.Worker, error) {
		var flag Flag
		if err := context.Get(flagName, &flag); err != nil {
			return nil, errors.Trace(err)
		}
		if !flag.Check() {
			return nil, ErrMissing
		}

		worker, err := inner(context)
		if err != nil {
			return nil, errors.Trace(err)
		}
		return worker, nil
	}
}
