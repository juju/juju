// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelgeneration

import (
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/permission"
)

var logger = loggo.GetLogger("juju.apiserver.modelgeneration")

// ModelGenerationAPI implements the ModelGeneration interface and is the concrete implementation
// of the API endpoint.
type ModelGenerationAPI struct {
	check             *common.BlockChecker
	authorizer        facade.Authorizer
	apiUser           names.UserTag
	isControllerAdmin bool
	model             GenerationModel
}

// NewModelGenerationFacade provides the signature required for facade registration.
func NewModelGenerationFacade(ctx facade.Context) (*ModelGenerationAPI, error) {
	authorizer := ctx.Auth()
	st := &modelGenerationStateShim{State: ctx.State()}
	model, err := st.Model()
	if err != nil {
		return nil, errors.Trace(err)
	}
	return NewModelGenerationAPI(st, authorizer, model)
}

// NewModelGenerationAPI creates a new API endpoint for dealing with model generations.
func NewModelGenerationAPI(
	st ModelGenerationState,
	authorizer facade.Authorizer,
	m GenerationModel,
) (*ModelGenerationAPI, error) {
	if !authorizer.AuthClient() {
		return nil, common.ErrPerm
	}
	// Since we know this is a user tag (because AuthClient is true),
	// we just do the type assertion to the UserTag.
	apiUser, _ := authorizer.GetAuthTag().(names.UserTag)
	// Pretty much all of the user manager methods have special casing for admin
	// users, so look once when we start and remember if the user is an admin.
	isAdmin, err := authorizer.HasPermission(permission.SuperuserAccess, st.ControllerTag())
	if err != nil {
		return nil, errors.Trace(err)
	}

	return &ModelGenerationAPI{
		authorizer:        authorizer,
		isControllerAdmin: isAdmin,
		apiUser:           apiUser,
		model:             m,
	}, nil
}

func (m *ModelGenerationAPI) hasAdminAccess(modelTag names.ModelTag) (bool, error) {
	canWrite, err := m.authorizer.HasPermission(permission.AdminAccess, modelTag)
	if errors.IsNotFound(err) {
		return false, nil
	}
	return canWrite, err
}

// AddGeneration adds a 'next' generation to the given model.
func (m *ModelGenerationAPI) AddGeneration(arg params.Entity) (params.ErrorResult, error) {
	result := params.ErrorResult{}
	modelTag, err := names.ParseModelTag(arg.Tag)
	if err != nil {
		return result, errors.Trace(err)
	}
	isModelAdmin, err := m.hasAdminAccess(modelTag)
	if !isModelAdmin && !m.isControllerAdmin {
		return result, common.ErrPerm
	}

	result.Error = common.ServerError(m.model.AddGeneration())
	return result, nil
}

// HasNextGeneration returns a true result if the input model has a "next"
// generation that has not yet been completed.
func (m *ModelGenerationAPI) HasNextGeneration(arg params.Entity) (params.BoolResult, error) {
	result := params.BoolResult{}
	modelTag, err := names.ParseModelTag(arg.Tag)
	if err != nil {
		return result, errors.Trace(err)
	}
	isModelAdmin, err := m.hasAdminAccess(modelTag)
	if !isModelAdmin && !m.isControllerAdmin {
		return result, common.ErrPerm
	}

	if has, err := m.model.HasNextGeneration(); err != nil {
		result.Error = common.ServerError(err)
	} else {
		result.Result = has
	}
	return result, nil
}

// AdvanceGeneration, adds the provided unit(s) and/or application(s) to
// the "next" generation.  If the generation can auto complete, it is
// made the "current" generation.
func (m *ModelGenerationAPI) AdvanceGeneration(arg params.AdvanceGenerationArg) (params.AdvanceGenerationResult, error) {
	modelTag, err := names.ParseModelTag(arg.Model.Tag)
	if err != nil {
		return params.AdvanceGenerationResult{}, errors.Trace(err)
	}
	isModelAdmin, err := m.hasAdminAccess(modelTag)
	if !isModelAdmin && !m.isControllerAdmin {
		return params.AdvanceGenerationResult{}, common.ErrPerm
	}

	generation, err := m.model.NextGeneration()
	if err != nil {
		return params.AdvanceGenerationResult{}, errors.Trace(err)
	}

	results := params.ErrorResults{
		Results: make([]params.ErrorResult, len(arg.Entities)),
	}
	for i, entity := range arg.Entities {
		tag, err := names.ParseTag(entity.Tag)
		if err != nil {
			results.Results[i].Error = common.ServerError(err)
			continue
		}
		switch tag.Kind() {
		case names.ApplicationTagKind:
			results.Results[i].Error = common.ServerError(generation.AssignAllUnits(tag.Id()))
		case names.UnitTagKind:
			results.Results[i].Error = common.ServerError(generation.AssignUnit(tag.Id()))
		default:
			results.Results[i].Error = common.ServerError(
				errors.Errorf("expected names.UnitTag or names.ApplicationTag, got %T", tag))
		}
		if err := generation.Refresh(); err != nil {
			return params.AdvanceGenerationResult{AdvanceResults: results}, errors.Trace(err)
		}
	}
	result := params.AdvanceGenerationResult{AdvanceResults: results}

	// Complete the generation if possible.
	completed, err := generation.AutoComplete()
	result.CompleteResult = params.BoolResult{
		Result: completed,
		Error:  common.ServerError(err),
	}
	return result, nil
}

// CancelGeneration cancels the 'next' generation if cancel
// criteria are met.
func (m *ModelGenerationAPI) CancelGeneration(arg params.Entity) (params.ErrorResult, error) {
	result := params.ErrorResult{}
	modelTag, err := names.ParseModelTag(arg.Tag)
	if err != nil {
		return result, errors.Trace(err)
	}
	isModelAdmin, err := m.hasAdminAccess(modelTag)
	if !isModelAdmin && !m.isControllerAdmin {
		return result, common.ErrPerm
	}

	generation, err := m.model.NextGeneration()
	if err != nil {
		return result, errors.Trace(err)
	}
	result.Error = common.ServerError(generation.MakeCurrent())
	return result, nil
}
