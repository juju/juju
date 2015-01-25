// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage

import (
	"github.com/juju/names"

	"github.com/juju/errors"
	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/feature"
	"github.com/juju/juju/state"
)

func init() {
	common.RegisterStandardFacadeForFeature("Storage", 1, NewAPI, feature.Storage)
}

var getState = func(st *state.State) storageAccess {
	return stateShim{st}
}

type StorageAPI interface {
	Show(entities params.Entities) (params.StorageShowResults, error)
}

// API implements the storage interface and is the concrete
// implementation of the api end point.
type API struct {
	storage    storageAccess
	authorizer common.Authorizer
}

// NewAPI returns a new storage API facade.
func NewAPI(
	st *state.State,
	resources *common.Resources,
	authorizer common.Authorizer,
) (*API, error) {
	if !authorizer.AuthClient() {
		return nil, common.ErrPerm
	}

	return &API{
		storage:    getState(st),
		authorizer: authorizer,
	}, nil
}

func (api *API) Show(entities params.Entities) (params.StorageShowResults, error) {
	all := make([]params.StorageShowResult, len(entities.Entities))
	for i, entity := range entities.Entities {
		all[i] = api.createStorageInstanceResult(entity.Tag)
	}
	return params.StorageShowResults{Results: all}, nil
}

func (api *API) createStorageInstanceResult(tag string) params.StorageShowResult {
	aTag, err := names.ParseTag(tag)
	if err != nil {
		return params.StorageShowResult{
			Error: params.ErrorResult{
				Error: common.ServerError(errors.Annotatef(common.ErrPerm, "getting %v", tag))},
		}
	}
	stateInstance, err := api.storage.StorageInstance(aTag.Id())
	if err != nil {
		return params.StorageShowResult{
			Error: params.ErrorResult{
				Error: common.ServerError(errors.Annotatef(common.ErrPerm, "getting %v", tag))},
		}
	}
	return params.StorageShowResult{Result: api.getStorageInstance(stateInstance)}
}

func (api *API) getStorageInstance(si state.StorageInstance) params.StorageInstance {
	return params.StorageInstance{
		OwnerTag:    si.Owner().String(),
		StorageTag:  si.Tag().String(),
		StorageName: si.StorageName(),
		Location:    "",
		TotalSize:   0,
	}
}
