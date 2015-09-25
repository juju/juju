// The uniter package implements the API interface used by the uniter
// worker. This file contains the API facade version 2.

package uniter

import (
	"github.com/juju/names"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/state"
)

func init() {
	common.RegisterStandardFacade("Uniter", 3, NewUniterAPIV3)
}

// UniterAPI implements the API version 3, used by the uniter worker.
type UniterAPIV3 struct {
	UniterAPIV2
}

func (u *UniterAPIV3) IsDynamicEndpoint(args params.DynamicEndpoint) (params.IsDynamicEndpointResults, error) {
	result := params.IsDynamicEndpointResults{
		Results: make([]params.IsDynamicEndpoint, len(args.Entities)),
	}
	canAccess, err := u.accessService()
	if err != nil {
		logger.Warningf("failed to check service access: %v", err)
		return params.IsDynamicEndpointResults{}, common.ErrPerm
	}

	for i, batch := range args.Entities {
		tag, err := names.ParseServiceTag(batch.Tag)
		if err != nil {
			result.Results[i].Error = common.ServerError(err)
			continue
		}
		if !canAccess(tag) {
			result.Results[i].Error = common.ServerError(common.ErrPerm)
			continue
		}
		service, err := u.getService(tag)
		if err != nil {
			result.Results[i].Error = common.ServerError(err)
			continue
		}
		dynamic := service.IsDynamicEndpoint(batch.Name, batch.Interface)
		result.Results[i].Dynamic = dynamic
		result.Results[i].Error = common.ServerError(nil)
	}

	return result, nil
}

func (u *UniterAPIV3) AddDynamicEndpoint(args params.DynamicEndpoint) (params.ErrorResults, error) {
	result := params.ErrorResults{
		Results: make([]params.ErrorResult, len(args.Entities)),
	}
	canAccess, err := u.accessService()
	if err != nil {
		logger.Warningf("failed to check service access: %v", err)
		return params.ErrorResults{}, common.ErrPerm
	}
	for i, batch := range args.Entities {
		tag, err := names.ParseServiceTag(batch.Tag)
		if err != nil {
			result.Results[i].Error = common.ServerError(err)
			continue
		}
		if !canAccess(tag) {
			result.Results[i].Error = common.ServerError(common.ErrPerm)
			continue
		}
		service, err := u.getService(tag)
		if err != nil {
			result.Results[i].Error = common.ServerError(err)
			continue
		}
		err = service.AddDynamicEndpoint(batch.Name, batch.Interface)
		result.Results[i].Error = common.ServerError(err)
	}
	return result, nil
}

// NewUniterAPIV3 creates a new instance of the Uniter API, version 2.
func NewUniterAPIV3(st *state.State, resources *common.Resources, authorizer common.Authorizer) (*UniterAPIV3, error) {
	baseAPI, err := NewUniterAPIV2(st, resources, authorizer)
	if err != nil {
		return nil, err
	}
	return &UniterAPIV3{
		UniterAPIV2: *baseAPI,
	}, nil
}
