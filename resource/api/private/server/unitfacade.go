package server

import (
	"github.com/juju/errors"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/resource"
	"github.com/juju/juju/resource/api"
	"github.com/juju/juju/resource/api/private"
)

type UnitDataStore interface {
	DownloadDataStore

	ListResources() ([]resource.Resource, error)
}

func NewUnitFacade(dataStore UnitDataStore) *UnitFacade {
	return &UnitFacade{
		dataStore: dataStore,
	}
}

type UnitFacade struct {
	dataStore UnitDataStore
}

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
