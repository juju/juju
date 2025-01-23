// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resource

import (
	"github.com/juju/errors"

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
// the application's resources that are out of date. Note that there must be
// a charm store resource for each of the application resources and
// vice-versa. If they are out of sync then an error is returned.
func (sr ApplicationResources) Updates() ([]resource.Resource, error) {
	storeResources, err := sr.alignStoreResources()
	if err != nil {
		return nil, errors.Trace(err)
	}

	var updates []resource.Resource
	for i, res := range sr.Resources {
		if res.Origin != resource.OriginStore {
			continue
		}
		csRes := storeResources[i]
		// If the revision is the same then all the other info must be.
		if res.Revision == csRes.Revision {
			continue
		}
		updates = append(updates, csRes)
	}
	return updates, nil
}

func (sr ApplicationResources) alignStoreResources() ([]resource.Resource, error) {
	if len(sr.RepositoryResources) > len(sr.Resources) {
		return nil, errors.Errorf("have more charm store resources than application resources")
	}
	if len(sr.RepositoryResources) < len(sr.Resources) {
		return nil, errors.Errorf("have fewer charm store resources than application resources")
	}

	var store []resource.Resource
	for _, res := range sr.Resources {
		found := false
		for _, chRes := range sr.RepositoryResources {
			if chRes.Name == res.Name {
				store = append(store, chRes)
				found = true
				break
			}
		}
		if !found {
			return nil, errors.Errorf("charm store resource %q not found", res.Name)
		}
	}
	return store, nil
}

// UnitResources conains the list of resources used by a unit.
type UnitResources struct {
	// Name is the name of the unit.
	Name coreunit.Name

	// Resources are the resource versions currently in use by this unit.
	Resources []Resource
}
