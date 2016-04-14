// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package util

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
