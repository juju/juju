// Copyright 2011, 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package environs

import (
	"fmt"

	"github.com/juju/errors"
)

const (
	// ErrAvailabilityZoneIndependent is an error that represents if the error
	// is independent of any particular availability zone. Juju uses this to
	// decide whether or not to attempt the failed operation in another
	// availability zone. Errors that conform to
	// Is(err, ErrAvailabilityZoneIndependent) will not be reattempted.
	ErrAvailabilityZoneIndependent = errors.ConstError("availability zone independent")

	// ErrNotBootstrapped reports that the given model is not bootstrapped.
	ErrNotBootstrapped = errors.ConstError("model is not bootstrapped")

	// ErrPartialInstances reports that the only some of the expected instance
	// were found.
	ErrPartialInstances = errors.ConstError("only some instances were found")
)

var (
	// ErrNotInstances represents and error for describing that no instances
	// were found.
	// NOTE: 2022-04-01 tlm This error carries some technical debt. Ideally it
	// would be nice to make this a ConstError but it's very unclear if this
	// error needs to also be represented as a NotFound error. In 2.9 we are
	// going to leave it as is but break it for 3.0.
	ErrNoInstances = fmt.Errorf("instances %w", errors.NotFound)
)

// ZoneIndependentError wraps err so that it satisfy
// Is(err, ErrAvailabilityZoneIndependent) and the errors.Locationer interface.
// If a nil error is provider then a nil error is returned.
func ZoneIndependentError(err error) error {
	return errors.SetLocation(
		errors.WithType(err, ErrAvailabilityZoneIndependent),
		1,
	)
}
