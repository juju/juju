// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage

import (
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
	Show(wanted params.StorageInstance) ([]params.StorageInstance, error)
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

func (api *API) Show(entities params.Entities) (params.StorageInstancesResult, error) {
	return api.storage.Show(entities)
}
