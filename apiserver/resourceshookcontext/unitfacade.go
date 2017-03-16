// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resourceshookcontext

import (
	"reflect"

	"github.com/juju/errors"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/resource"
	"github.com/juju/juju/resource/api"
	"github.com/juju/juju/state"
)

func init() {
	common.RegisterHookContextFacade(
		"ResourcesHookContext", 1,
		newHookContextFacade,
		reflect.TypeOf(&UnitFacade{}),
	)
}

func newHookContextFacade(st *state.State, unit *state.Unit) (interface{}, error) {
	res, err := st.Resources()
	if err != nil {
		return nil, errors.Trace(err)
	}
	return NewUnitFacade(&resourcesUnitDataStore{res, unit}), nil
}

// resourcesUnitDatastore is a shim to elide serviceName from
// ListResources.
type resourcesUnitDataStore struct {
	resources state.Resources
	unit      *state.Unit
}

// ListResources implements resource/api/private/server.UnitDataStore.
func (ds *resourcesUnitDataStore) ListResources() (resource.ServiceResources, error) {
	return ds.resources.ListResources(ds.unit.ApplicationName())
}

// UnitDataStore exposes the data storage functionality needed here.
// All functionality is tied to the unit's application.
type UnitDataStore interface {
	// ListResources lists all the resources for the application.
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
// resource names (for the implicit application). If any one is missing then
// the corresponding result is set with errors.NotFound.
func (uf UnitFacade) GetResourceInfo(args params.ListUnitResourcesArgs) (params.UnitResourcesResult, error) {
	var r params.UnitResourcesResult
	r.Resources = make([]params.UnitResourceResult, len(args.ResourceNames))

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
