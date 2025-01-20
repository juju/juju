// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resource

import (
	coreunit "github.com/juju/juju/core/unit"
	"github.com/juju/juju/internal/charm/resource"
)

// ApplicationResources contains the list of resources for the application and all its
// units.
type ApplicationResources struct {
	// Resources are the current version of the resource for the application that
	// resource-get will retrieve.
	Resources []Resource

	// RepositoryResources provides the resource info from the charm
	// store for each of the application's resources. The information from
	// the charm store is current as of the last time the charm store
	// was polled. Each entry here corresponds to the same indexed entry
	// in the Resources field.
	RepositoryResources []resource.Resource

	// UnitResources reports the currently-in-use version of resources for each
	// unit.
	UnitResources []UnitResources
}

// Updates returns the list of charm store resources corresponding to
// the application's resources that are out of date. If there is a charm
// store resource with a different revision than the one used into the
// application, it will be returned.
// Any charm store resources with the same revision number from the
// corresponding application resources will be filtered out.
func (sr ApplicationResources) Updates() ([]resource.Resource, error) {
	storeResources := map[string]resource.Resource{}
	for _, res := range sr.RepositoryResources {
		storeResources[res.Name] = res
	}

	var updates []resource.Resource
	for _, res := range sr.Resources {
		if res.Origin != resource.OriginStore {
			continue
		}
		csRes, ok := storeResources[res.Name]
		// If the revision is the same then all the other info must be.
		if !ok || res.Revision == csRes.Revision {
			continue
		}
		updates = append(updates, csRes)
	}
	return updates, nil
}

// UnitResources contains the list of resources used by a unit.
type UnitResources struct {
	// Name is the name of the unit.
	Name coreunit.Name

	// Resources are the resource versions currently in use by this unit.
	Resources []Resource
}
