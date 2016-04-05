// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common

import (
	"github.com/juju/errors"
	"github.com/juju/juju/worker"
	"github.com/juju/juju/worker/dependency"
	"github.com/juju/juju/worker/fortress"
)

// Spec expresses a convenient set of tweaks to a dependency.Manifold.
// It has a single Decorate method, which returns a new Manifold with
// the following modifications:
type Spec struct {

	// Occupy, if empty, is ignored. Otherwise it's added to Inputs,
	// if not already present, and the Start func is wrapped to
	// invoke the worker only if Occupy points to a fortress.Guest;
	// and both to ensure that the worker can only run while that
	// fortress is unlocked, and that the fortress cannot be locked
	// down while the worker is running.
	//
	// NOTE: this effectively involves acquiring a lock, and demands
	// exactly as much care and attention as any other locking
	// operation. Lock acquisition will be a decorated manifold's
	// last act before starting its worker -- we will make a good
	// faith attempt to minimise the time it's held for -- but if
	// you start chaining Specs that Occupy, you are inviting
	// deadlock vampires into the house and it is guaranteed to end
	// badly.
	Occupy string

	// Flags are each added to Inputs, if not already present, and
	// the Start func is wrapped to only run if each name points to
	// a dependency.Flag() whose Check() succeeds.
	Flags []string

	// Filter unconditionally sets a single error filter for the
	// manifold. They could plausibly be chained instead, but use
	// of multiple filters is likely to signify that you're doing
	// something wrong anyway: the hosting Engine has an overly-
	// enthusiastic IsFatal, and/or the responsibility for stopping
	// the Engine is too widely distributed across its manifolds.
	Filter dependency.FilterFunc
}

// Decorate returns a copy of base with changes made according to spec.
func (spec Spec) Decorate(base dependency.Manifold) dependency.Manifold {
	manifold := base
	if spec.Occupy != "" {
		manifold.Inputs = maybeAdd(manifold.Inputs, spec.Occupy)
		manifold.Start = occupyStart(manifold.Start, spec.Occupy)
	}
	for _, name := range spec.Flags {
		manifold.Inputs = maybeAdd(manifold.Inputs, name)
		manifold.Start = flagStart(manifold.Start, name)
	}
	manifold.Filter = spec.Filter
	return manifold
}

func maybeAdd(inputs []string, add string) []string {
	for _, input := range inputs {
		if input == add {
			return inputs
		}
	}
	result := make([]string, len(inputs)+1)
	copy(result, inputs)
	return append(result, add)
}

func flagStart(inner dependency.StartFunc, name string) dependency.StartFunc {
	return func(context dependency.Context) (worker.Worker, error) {
		var flag dependency.Flag
		if err := context.Get(name, &flag); err != nil {
			return nil, errors.Trace(err)
		}
		if !flag.Check() {
			return nil, dependency.ErrMissing
		}
		return inner(context)
	}
}

func occupyStart(inner dependency.StartFunc, name string) dependency.StartFunc {
	return func(context dependency.Context) (worker.Worker, error) {
		var guest fortress.Guest
		if err := context.Get(name, &guest); err != nil {
			return nil, errors.Trace(err)
		}
		start := func() (worker.Worker, error) {
			return inner(context)
		}
		return fortress.Occupy(guest, start, context.Abort())
	}
}
