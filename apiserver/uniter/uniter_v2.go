// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// The uniter package implements the API interface used by the uniter
// worker. This file contains the API facade version 2.

package uniter

import (
	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/state"
)

func init() {
	common.RegisterStandardFacade("Uniter", 2, NewUniterAPIV2)
}

// UniterAPI implements the API version 2, used by the uniter worker.
type UniterAPIV2 struct {
	UniterAPIV1
	StorageAPI
}

// NewUniterAPIV2 creates a new instance of the Uniter API, version 2.
func NewUniterAPIV2(st *state.State, resources *common.Resources, authorizer common.Authorizer) (*UniterAPIV2, error) {
	baseAPI, err := NewUniterAPIV1(st, resources, authorizer)
	if err != nil {
		return nil, err
	}
	storageAPI, err := newStorageAPI(getStorageState(st), resources, baseAPI.accessUnit)
	if err != nil {
		return nil, err
	}
	return &UniterAPIV2{
		UniterAPIV1: *baseAPI,
		StorageAPI:  *storageAPI,
	}, nil
}
