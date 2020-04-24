// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resourceshookcontext

import (
	"github.com/juju/errors"
	"github.com/juju/names/v4"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/resource"
	"github.com/juju/juju/resource/api"
	"github.com/juju/juju/state"
)

// NewHookContextFacade adapts NewUnitFacade for facade registration.
func NewHookContextFacade(st *state.State, unit *state.Unit) (interface{}, error) {
	res, err := st.Resources()
	if err != nil {
		return nil, errors.Trace(err)
	}
	return NewUnitFacade(&resourcesUnitDataStore{res, unit}), nil
}

// NewStateFacade provides the signature to register this resource facade
func NewStateFacade(ctx facade.Context) (*UnitFacade, error) {
	authorizer := ctx.Auth()
	st := ctx.State()

	if !authorizer.AuthUnitAgent() && !authorizer.AuthApplicationAgent() {
		return nil, common.ErrPerm
	}

	var (
		unit *state.Unit
		err  error
	)
	switch tag := authorizer.GetAuthTag().(type) {
	case names.UnitTag:
		unit, err = st.Unit(tag.Id())
		if err != nil {
			return nil, errors.Trace(err)
		}
	case names.ApplicationTag:
		// Allow application access for K8s units. As they are all homogeneous any of the units will suffice.
		app, err := st.Application(tag.Id())
		if err != nil {
			return nil, errors.Trace(err)
		}
		allUnits, err := app.AllUnits()
		if err != nil {
			return nil, errors.Trace(err)
		}
		if len(allUnits) <= 0 {
			return nil, errors.Errorf("failed to get units for app: %s", app.Name())
		}
		unit = allUnits[0]
	default:
		return nil, errors.Errorf("expected names.UnitTag or names.ApplicationTag, got %T", tag)
	}

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
func (ds *resourcesUnitDataStore) ListResources() (resource.ApplicationResources, error) {
	return ds.resources.ListResources(ds.unit.ApplicationName())
}

// UnitDataStore exposes the data storage functionality needed here.
// All functionality is tied to the unit's application.
type UnitDataStore interface {
	// ListResources lists all the resources for the application.
	ListResources() (resource.ApplicationResources, error)
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
