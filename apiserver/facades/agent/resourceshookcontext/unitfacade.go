// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resourceshookcontext

import (
	"context"

	"github.com/juju/errors"

	"github.com/juju/juju/api/client/resources"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	coreresources "github.com/juju/juju/core/resources"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
)

// resourcesUnitDatastore is a shim to elide serviceName from
// ListResources.
type resourcesUnitDataStore struct {
	resources state.Resources
	unit      *state.Unit
}

// ListResources implements resource/api/private/server.UnitDataStore.
func (ds *resourcesUnitDataStore) ListResources() (coreresources.ApplicationResources, error) {
	return ds.resources.ListResources(ds.unit.ApplicationName())
}

// UnitDataStore exposes the data storage functionality needed here.
// All functionality is tied to the unit's application.
type UnitDataStore interface {
	// ListResources lists all the resources for the application.
	ListResources() (coreresources.ApplicationResources, error)
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
// resource names (for the implicit application). If any one is missing then
// the corresponding result is set with errors.NotFound.
func (uf UnitFacade) GetResourceInfo(ctx context.Context, args params.ListUnitResourcesArgs) (params.UnitResourcesResult, error) {
	var r params.UnitResourcesResult
	r.Resources = make([]params.UnitResourceResult, len(args.ResourceNames))

	foundResources, err := uf.DataStore.ListResources()
	if err != nil {
		r.Error = apiservererrors.ServerError(err)
		return r, nil
	}

	for i, name := range args.ResourceNames {
		res, ok := lookUpResource(name, foundResources.Resources)
		if !ok {
			r.Resources[i].Error = apiservererrors.ServerError(errors.NotFoundf("resource %q", name))
			continue
		}

		r.Resources[i].Resource = resources.Resource2API(res)
	}
	return r, nil
}

func lookUpResource(name string, resources []coreresources.Resource) (coreresources.Resource, bool) {
	for _, res := range resources {
		if name == res.Name {
			return res, true
		}
	}
	return coreresources.Resource{}, false
}
