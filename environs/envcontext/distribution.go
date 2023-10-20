// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package envcontext

import "github.com/juju/juju/core/instance"

// Distributor is an interface that may be used to distribute
// application units across instances for high availability.
type Distributor interface {
	// DistributeInstances takes a set of clean, empty instances,
	// a distribution group, and list of zones to limit the consideration to.
	// If the input zone collection has no elements, then all availability
	// zones are considered when attempting distribution.
	// It returns the subset of candidates that the policy will allow to enter
	// the distribution group.
	//
	// The AssignClean and AssignCleanEmpty unit assignment policies will
	// attempt to assign a unit to each of the resulting instances until one is
	// successful. If no instances can be assigned to (e.g. because of
	// concurrent deployments), then a new machine will be allocated.
	DistributeInstances(
		ctx ProviderCallContext, candidates, distributionGroup []instance.Id, limitZones []string,
	) ([]instance.Id, error)
}
