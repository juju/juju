// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common

import (
	"github.com/juju/loggo"

	"launchpad.net/juju-core/constraints"
	"launchpad.net/juju-core/environs"
)

// InstanceTypeUnsupported logs a warning if cons contains an instance type value.
func InstanceTypeUnsupported(logger loggo.Logger, e environs.Environ, cons constraints.Value) {
	if cons.HasInstanceType() {
		logger.Warningf("instance-type constraint %q not supported for %s provider %q",
			*cons.InstanceType, e.Config().Type(), e.Name())
	}
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
	// we used those, ignoring instance constraint.
	if !constraints.IsEmpty(&consWithoutInstType) {
		logger.Warningf("instance-type constraint %q ignored since other constraints are specified", cons.InstanceType)
		return consWithoutInstType
	}
	// If we are here, cons contains just an instance type constraint value.
	return cons
}
