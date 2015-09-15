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

func (u *UniterAPIV3) AddCharmRelation(args params.AddCharmRelation) (params.ErrorResults, error) {
	result := params.ErrorResults{
		Results: make([]params.ErrorResult, len(args.Entities)),
	}
	canAccess, err := u.accessUnit()
	if err != nil {
		logger.Warningf("failed to check unit access: %v", err)
		return params.ErrorResults{}, common.ErrPerm
	}
	for i, batch := range args.Entities {
		tag, err := names.ParseUnitTag(batch.Tag)
		if err != nil {
			result.Results[i].Error = common.ServerError(err)
			continue
		}
		if !canAccess(tag) {
			result.Results[i].Error = common.ServerError(common.ErrPerm)
			continue
		}
		unit, err := u.getUnit(tag)
		if err != nil {
			result.Results[i].Error = common.ServerError(err)
			continue
		}
		err = unit.AddCharmRelation(batch.Name, batch.Interface)
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
