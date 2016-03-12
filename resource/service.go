// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resource

import (
	"github.com/juju/errors"
	"gopkg.in/juju/charm.v6-unstable/resource"
)

// ServiceResources contains the list of resources for the service and all its
// units.
type ServiceResources struct {
	// Resources are the current version of the resource for the service that
	// resource-get will retrieve.
	Resources []Resource

	// CharmStoreResources provides the resource info from the charm
	// store for each of the service's resources. The information from
	// the charm store is current as of the last time the charm store
	// was polled. Each entry here corresponds to the same indexed entry
	// in the Resources field.
	CharmStoreResources []resource.Resource

	// UnitResources reports the currenly-in-use version of resources for each
	// unit.
	UnitResources []UnitResources
}

// Updates returns the list of charm store resources corresponding to
// the service's resources that are out of date. Note that there must be
// a charm store resource for each of the service resources and
// vice-versa. If they are out of sync then an error is returned.
func (sr ServiceResources) Updates() ([]resource.Resource, error) {
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

func (sr ServiceResources) alignStoreResources() ([]resource.Resource, error) {
	if len(sr.CharmStoreResources) > len(sr.Resources) {
		return nil, errors.Errorf("have more charm store resources than service resources")
	}
	if len(sr.CharmStoreResources) < len(sr.Resources) {
		return nil, errors.Errorf("have fewer charm store resources than service resources")
	}

	var store []resource.Resource
	for _, res := range sr.Resources {
		found := false
		for _, chRes := range sr.CharmStoreResources {
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
