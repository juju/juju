// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage

import (
	"github.com/juju/errors"
	"github.com/juju/names"

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
		instance, err := api.oneStorageInstance(entity.Tag)
		if err == nil {
			all[i].Result = instance
		} else {
			err := errors.Annotatef(err, "getting %v", entity.Tag)
			all[i].Error = common.ServerError(err)
		}
	}
	return params.StorageShowResults{Results: all}, nil
}

func (api *API) oneStorageInstance(tag string) (params.StorageInstance, error) {
	aTag, err := names.ParseStorageTag(tag)
	if err != nil {
		return params.StorageInstance{}, common.ErrPerm
	}
	stateStorageInstance, err := api.storage.StorageInstance(aTag)
	if err != nil {
		return params.StorageInstance{}, common.ErrPerm
	}
	// TODO(axw) get the avail/total size for the storage instance.
	// TODO(axw) return attachments with the instance, including location.
	return params.StorageInstance{
		StorageTag: stateStorageInstance.Tag().String(),
		OwnerTag:   stateStorageInstance.Owner().String(),
		Kind:       params.StorageKind(stateStorageInstance.Kind()),
	}, nil
}
