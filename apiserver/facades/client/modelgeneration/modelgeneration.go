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
	"github.com/juju/juju/core/model"
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

// AddBranch adds a new branch with the input name to the model.
func (m *API) AddBranch(arg params.BranchArg) (params.ErrorResult, error) {
	result := params.ErrorResult{}
	modelTag, err := names.ParseModelTag(arg.Model.Tag)
	if err != nil {
		return result, errors.Trace(err)
	}
	isModelAdmin, err := m.hasAdminAccess(modelTag)
	if !isModelAdmin && !m.isControllerAdmin {
		return result, common.ErrPerm
	}

	if err := model.ValidateBranchName(arg.BranchName); err != nil {
		result.Error = common.ServerError(err)
	} else {
		result.Error = common.ServerError(m.model.AddBranch(arg.BranchName, m.apiUser.Name()))
	}
	return result, nil
}

// TrackBranch marks the input units and/or applications as tracking the input
// branch, causing them to realise changes made under that branch.
func (m *API) TrackBranch(arg params.BranchTrackArg) (params.ErrorResults, error) {
	modelTag, err := names.ParseModelTag(arg.Model.Tag)
	if err != nil {
		return params.ErrorResults{}, errors.Trace(err)
	}
	isModelAdmin, err := m.hasAdminAccess(modelTag)
	if !isModelAdmin && !m.isControllerAdmin {
		return params.ErrorResults{}, common.ErrPerm
	}

	branch, err := m.model.Branch(arg.BranchName)
	if err != nil {
		return params.ErrorResults{}, errors.Trace(err)
	}

	result := params.ErrorResults{
		Results: make([]params.ErrorResult, len(arg.Entities)),
	}
	for i, entity := range arg.Entities {
		tag, err := names.ParseTag(entity.Tag)
		if err != nil {
			result.Results[i].Error = common.ServerError(err)
			continue
		}
		switch tag.Kind() {
		case names.ApplicationTagKind:
			result.Results[i].Error = common.ServerError(branch.AssignAllUnits(tag.Id()))
		case names.UnitTagKind:
			result.Results[i].Error = common.ServerError(branch.AssignUnit(tag.Id()))
		default:
			result.Results[i].Error = common.ServerError(
				errors.Errorf("expected names.UnitTag or names.ApplicationTag, got %T", tag))
		}
	}
	return result, nil
}

// CommitBranch commits the input branch, making its changes applicable to
// the whole model and marking it complete.
func (m *API) CommitBranch(arg params.BranchArg) (params.IntResult, error) {
	result := params.IntResult{}

	modelTag, err := names.ParseModelTag(arg.Model.Tag)
	if err != nil {
		return result, errors.Trace(err)
	}
	isModelAdmin, err := m.hasAdminAccess(modelTag)
	if !isModelAdmin && !m.isControllerAdmin {
		return result, common.ErrPerm
	}

	generation, err := m.model.Branch(arg.BranchName)
	if err != nil {
		return result, errors.Trace(err)
	}

	if genId, err := generation.Commit(m.apiUser.Name()); err != nil {
		result.Error = common.ServerError(err)
	} else {
		result.Result = genId
	}
	return result, nil
}

// BranchInfo will return details of branch identified by the input argument,
// including units on the branch and the configuration disjoint with the
// master generation.
// An error is returned if no in-flight branch matching in input is found.
func (m *API) BranchInfo(arg params.BranchArg) (params.GenerationResult, error) {
	modelTag, err := names.ParseModelTag(arg.Model.Tag)
	if err != nil {
		return params.GenerationResult{}, errors.Trace(err)
	}
	isModelAdmin, err := m.hasAdminAccess(modelTag)
	if !isModelAdmin && !m.isControllerAdmin {
		return params.GenerationResult{}, common.ErrPerm
	}

	gen, err := m.model.Branch(arg.BranchName)
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
		cfgCurrent, err := app.CharmConfig(model.GenerationMaster)
		if err != nil {
			return generationInfoError(err)
		}
		cfgNext, err := app.CharmConfig(arg.BranchName)
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
		BranchName:   gen.BranchName(),
		Created:      gen.Created(),
		CreatedBy:    gen.CreatedBy(),
		Applications: apps,
	}}, nil
}

// HasActiveBranch returns a true result if the input model has an "in-flight"
// branch matching the input name.
func (m *API) HasActiveBranch(arg params.BranchArg) (params.BoolResult, error) {
	result := params.BoolResult{}
	modelTag, err := names.ParseModelTag(arg.Model.Tag)
	if err != nil {
		return result, errors.Trace(err)
	}
	isModelAdmin, err := m.hasAdminAccess(modelTag)
	if !isModelAdmin && !m.isControllerAdmin {
		return result, common.ErrPerm
	}

	if _, err := m.model.Branch(arg.BranchName); err != nil {
		if err != nil {
			if errors.IsNotFound(err) {
				result.Result = false
			} else {
				result.Error = common.ServerError(err)
			}
		}
	} else {
		result.Result = true
	}
	return result, nil
}

func generationInfoError(err error) (params.GenerationResult, error) {
	return params.GenerationResult{Error: common.ServerError(err)}, nil
}
