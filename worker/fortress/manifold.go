// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package fortress

import (
	"github.com/juju/errors"
	"github.com/juju/worker/v2"
	"github.com/juju/worker/v2/dependency"
)

// Manifold returns a dependency.Manifold that runs a fortress.
//
// Clients should access the fortress resource via Guard and/or Guest pointers.
// Guest.Visit calls will block until a Guard.Unlock call is made; Guard.Lockdown
// calls will block new Guest.Visits and wait until all active Visits complete.
//
// If multiple clients act as guards, the fortress' state at any time will be
// determined by whichever guard last ran an operation; that is to say, it will
// be impossible to reliably tell from outside. So please don't do that.
func Manifold() dependency.Manifold {
	return dependency.Manifold{
		Start: func(_ dependency.Context) (worker.Worker, error) {
			return newFortress(), nil
		},
		Output: func(in worker.Worker, out interface{}) error {
			inFortress, _ := in.(*fortress)
			if inFortress == nil {
				return errors.Errorf("in should be %T; is %T", inFortress, in)
			}
			switch outPointer := out.(type) {
			case *Guard:
				*outPointer = inFortress
			case *Guest:
				*outPointer = inFortress
			default:
				return errors.Errorf("out should be *fortress.Guest or *fortress.Guard; is %T", out)
			}
			return nil
		},
	}
}
