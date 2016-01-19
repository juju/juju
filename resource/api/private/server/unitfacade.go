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

// UnitDataStore exposes the data storage functionality needed here.
// All functionality is tied to the unit's service.
type UnitDataStore interface {
	DownloadDataStore

	// ListResources lists all the resources for the service.
	ListResources() ([]resource.Resource, error)
}

// NewUnitFacade returns the resources portion of the uniter's API facade.
func NewUnitFacade(dataStore UnitDataStore) *UnitFacade {
	return &UnitFacade{
		dataStore: dataStore,
	}
}

// UnitFacade is the resources portion of the uniter's API facade.
type UnitFacade struct {
	dataStore UnitDataStore
}

// GetResourceInfo returns the resource info for each of the given
// resource names (for the implicit service). If any one is missing then
// the corresponding result is set with errors.NotFound.
func (uf UnitFacade) GetResourceInfo(args private.ListResourcesArgs) (private.ResourcesResult, error) {
	var r private.ResourcesResult
	r.Resources = make([]private.ResourceResult, len(args.ResourceNames))

	resources, err := uf.dataStore.ListResources()
	if err != nil {
		r.Error = common.ServerError(err)
	}

	for i, name := range args.ResourceNames {
		r.Resources[i].Error = common.ServerError(errors.NotFoundf("resource %q", name))
		for _, res := range resources {
			if name == res.Name {
				r.Resources[i].Resource = api.Resource2API(res)
				r.Resources[i].Error = nil
				break
			}
		}
	}
	return r, nil
}
