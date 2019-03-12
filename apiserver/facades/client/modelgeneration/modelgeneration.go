// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelgeneration

import (
	"github.com/juju/errors"
	"github.com/juju/juju/core/model"
	"github.com/juju/loggo"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/permission"
)

var logger = loggo.GetLogger("juju.apiserver.modelgeneration")

// API implements the ModelGenerationAPI interface and is the concrete implementation
// of the API endpoint.
type API struct {
	check             *common.BlockChecker
	authorizer        facade.Authorizer
	apiUser           names.UserTag
	isControllerAdmin bool
	st                State
	model             Model
}

// NewModelGenerationFacade provides the signature required for facade registration.
func NewModelGenerationFacade(ctx facade.Context) (*API, error) {
	authorizer := ctx.Auth()
	st := &modelGenerationStateShim{State: ctx.State()}
	m, err := st.Model()
	if err != nil {
		return nil, errors.Trace(err)
	}
	return NewModelGenerationAPI(st, authorizer, m)
}

// NewModelGenerationAPI creates a new API endpoint for dealing with model generations.
func NewModelGenerationAPI(
	st State,
	authorizer facade.Authorizer,
	m Model,
) (*API, error) {
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

	return &API{
		authorizer:        authorizer,
		isControllerAdmin: isAdmin,
		apiUser:           apiUser,
		st:                st,
		model:             m,
	}, nil
}

func (m *API) hasAdminAccess(modelTag names.ModelTag) (bool, error) {
	canWrite, err := m.authorizer.HasPermission(permission.AdminAccess, modelTag)
	if errors.IsNotFound(err) {
		return false, nil
	}
	return canWrite, err
}

// AddGeneration adds a "next" generation to the given model.
func (m *API) AddGeneration(arg params.Entity) (params.ErrorResult, error) {
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
func (m *API) HasNextGeneration(arg params.Entity) (params.BoolResult, error) {
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
func (m *API) AdvanceGeneration(arg params.AdvanceGenerationArg) (params.AdvanceGenerationResult, error) {
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

// CancelGeneration cancels the "next" generation if cancel criteria are met.
func (m *API) CancelGeneration(arg params.Entity) (params.ErrorResult, error) {
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

// GenerationInfo will return details of the "next" generation,
// including units on the generation and the configuration disjoint with the
// current generation.
// An error is returned if there is no active "next" generation.
func (m *API) GenerationInfo(arg params.Entity) (params.GenerationResult, error) {
	modelTag, err := names.ParseModelTag(arg.Tag)
	if err != nil {
		return params.GenerationResult{}, errors.Trace(err)
	}
	isModelAdmin, err := m.hasAdminAccess(modelTag)
	if !isModelAdmin && !m.isControllerAdmin {
		return params.GenerationResult{}, common.ErrPerm
	}

	gen, err := m.model.NextGeneration()
	if err != nil {
		return generationInfoError(err)
	}

	var apps []params.GenerationApplication
	for appName, units := range gen.AssignedUnits() {
		app, err := m.st.Application(appName)
		if err != nil {
			return generationInfoError(err)
		}

		// TODO (manadart 2019-02-22): As more aspects are made generational,
		// each should go into its own method - charm, resources etc.
		cfgCurrent, err := app.CharmConfig(model.GenerationCurrent)
		if err != nil {
			return generationInfoError(err)
		}
		cfgNext, err := app.CharmConfig(model.GenerationNext)
		if err != nil {
			return generationInfoError(err)
		}
		cfgDelta := make(map[string]interface{})
		for k, v := range cfgNext {
			if cfgCurrent[k] != v {
				cfgDelta[k] = v
			}
		}

		genAppDelta := params.GenerationApplication{
			ApplicationName: appName,
			Units:           units,
			ConfigChanges:   cfgDelta,
		}
		apps = append(apps, genAppDelta)
	}

	return params.GenerationResult{Generation: params.Generation{
		Created:      gen.Created(),
		Applications: apps,
	}}, nil
}

func generationInfoError(err error) (params.GenerationResult, error) {
	return params.GenerationResult{Error: common.ServerError(err)}, nil
}
