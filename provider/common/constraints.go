// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common

import (
	"github.com/juju/loggo"

	"launchpad.net/juju-core/constraints"
	"launchpad.net/juju-core/environs"
)

// ValidateConstraints uses the default constraints WithFallbacks method to combine cons with envCons
// and logs a warning if the resulting combined constraints contains an instance type value.
// It is used by providers which do not support instance type constraints.
func ValidateConstraints(
	logger loggo.Logger, e environs.Environ, cons, envCons constraints.Value) (constraints.Value, error) {

	combinedCons := cons.WithFallbacks(envCons)
	if combinedCons.HasInstanceType() {
		logger.Warningf("instance-type constraint %q not supported for %s provider %q",
			*cons.InstanceType, e.Config().Type(), e.Name())
	}
	return combinedCons, nil
}

// ImageMatchConstraint returns a constrains.Value derived from cons according
// to whether InstanceType is specified as a constraint.
func ImageMatchConstraint(cons constraints.Value) constraints.Value {
	// No InstanceType specified, return the original constraint.
	if !cons.HasInstanceType() {
		return cons
	}
	consWithoutInstType := cons
	consWithoutInstType.InstanceType = nil
	// If the original constraint has attributes besides instances constraint,
	// we use those, ignoring instance constraint.
	if !constraints.IsEmpty(&consWithoutInstType) {
		logger.Warningf("instance-type constraint %q ignored since other constraints are specified", cons.InstanceType)
		return consWithoutInstType
	}
	// If we are here, cons contains just an instance type constraint value.
	return cons
}
