// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package instance

// Distributor is an interface that may be used to distribute
// application units across instances for high availability.
type Distributor interface {
	// DistributeInstance takes a set of clean, empty
	// instances, and a distribution group, and returns
	// the subset of candidates which the policy will
	// allow entry into the distribution group.
	//
	// The AssignClean and AssignCleanEmpty unit
	// assignment policies will attempt to assign a
	// unit to each of the resulting instances until
	// one is successful. If no instances can be assigned
	// to (e.g. because of concurrent deployments), then
	// a new machine will be allocated.
	DistributeInstances(candidates, distributionGroup []Id) ([]Id, error)
}
