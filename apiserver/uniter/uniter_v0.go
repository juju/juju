// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// The uniter package implements the API interface used by the uniter
// worker. This file contains the API facade version 0.
package uniter

import (
	"github.com/juju/names"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/state"
)

func init() {
	common.RegisterStandardFacade("Uniter", 0, NewUniterAPIV0)
}

// UniterAPIV0 implements the API facade version 0, used by the uniter
// worker.
type UniterAPIV0 struct {
	uniterBaseAPI
}

// NewUniterAPIV0 creates a new instance of the Uniter API, version 0.
func NewUniterAPIV0(st *state.State, resources *common.Resources, authorizer common.Authorizer) (*UniterAPIV0, error) {
	baseAPI, err := newUniterBaseAPI(st, resources, authorizer)
	if err != nil {
		return nil, err
	}
	return &UniterAPIV0{
		uniterBaseAPI: *baseAPI,
	}, nil
}

// GetOwnerTag returns the user tag of the owner of the first given
// service tag in args.
//
// NOTE: This is obsolete and is replaced by ServiceOwner in APIV1,
// which should be used instead. This method is not propely handling
// multiple tags and does not check for permissions. See also
// http://pad.lv/1270795.
func (u *UniterAPIV0) GetOwnerTag(args params.Entities) (params.StringResult, error) {
	var nothing params.StringResult
	tag, err := names.ParseServiceTag(args.Entities[0].Tag)
	if err != nil {
		return nothing, common.ErrPerm
	}
	service, err := u.getService(tag)
	if err != nil {
		return nothing, err
	}
	return params.StringResult{
		Result: service.GetOwnerTag(),
	}, nil
}
