// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package server

import (
	"github.com/juju/errors"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/resource"
	"github.com/juju/juju/resource/api"
	"github.com/juju/juju/resource/api/private"
)

// FacadeVersion is the version of the current API facade.
// (We start at 1 to distinguish from the default value.)
const FacadeVersion = 1

// UnitDataStore exposes the data storage functionality needed here.
// All functionality is tied to the unit's service.
type UnitDataStore interface {
	// ListResources lists all the resources for the service.
	ListResources() (resource.ServiceResources, error)
}

// NewUnitFacade returns the resources portion of the uniter's API facade.
func NewUnitFacade(dataStore UnitDataStore) *UnitFacade {
	return &UnitFacade{
		DataStore: dataStore,
	}
}

// UnitFacade is the resources portion of the uniter's API facade.
type UnitFacade struct {
	//DataStore is the data store used by the facade.
	DataStore UnitDataStore
}

// GetResourceInfo returns the resource info for each of the given
// resource names (for the implicit service). If any one is missing then
// the corresponding result is set with errors.NotFound.
func (uf UnitFacade) GetResourceInfo(args private.ListResourcesArgs) (private.ResourcesResult, error) {
	var r private.ResourcesResult
	r.Resources = make([]private.ResourceResult, len(args.ResourceNames))

	resources, err := uf.DataStore.ListResources()
	if err != nil {
		r.Error = common.ServerError(err)
		return r, nil
	}

	for i, name := range args.ResourceNames {
		res, ok := lookUpResource(name, resources.Resources)
		if !ok {
			r.Resources[i].Error = common.ServerError(errors.NotFoundf("resource %q", name))
			continue
		}

		r.Resources[i].Resource = api.Resource2API(res)
	}
	return r, nil
}

func lookUpResource(name string, resources []resource.Resource) (resource.Resource, bool) {
	for _, res := range resources {
		if name == res.Name {
			return res, true
		}
	}
	return resource.Resource{}, false
}
