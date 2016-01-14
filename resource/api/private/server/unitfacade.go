package server

import (
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

func (uf UnitFacade) GetResourceInfo(args private.ListResourcesArgs) (api.ResourcesResult, error) {

	var r api.ResourcesResult
	r.Resources = make([]api.Resource, len(args.ResourceNames))

	resources, err := uf.dataStore.ListResources()
	if err != nil {
		api.SetResultError(&r, err)
	}

	for i, res := range resources {
		r.Resources[i] = api.Resource2API(res)
	}
	return r, nil
}
