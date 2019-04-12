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
	st := &stateShim{State: ctx.State()}
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

func (api *API) hasAdminAccess() (bool, error) {
	canWrite, err := api.authorizer.HasPermission(permission.AdminAccess, api.model.ModelTag())
	if errors.IsNotFound(err) {
		return false, nil
	}
	return canWrite, err
}

// AddBranch adds a new branch with the input name to the model.
func (api *API) AddBranch(arg params.BranchArg) (params.ErrorResult, error) {
	result := params.ErrorResult{}
	isModelAdmin, err := api.hasAdminAccess()
	if err != nil {
		return result, errors.Trace(err)
	}
	if !isModelAdmin && !api.isControllerAdmin {
		return result, common.ErrPerm
	}

	if err := model.ValidateBranchName(arg.BranchName); err != nil {
		result.Error = common.ServerError(err)
	} else {
		result.Error = common.ServerError(api.model.AddBranch(arg.BranchName, api.apiUser.Name()))
	}
	return result, nil
}

// TrackBranch marks the input units and/or applications as tracking the input
// branch, causing them to realise changes made under that branch.
func (api *API) TrackBranch(arg params.BranchTrackArg) (params.ErrorResults, error) {
	isModelAdmin, err := api.hasAdminAccess()
	if err != nil {
		return params.ErrorResults{}, errors.Trace(err)
	}
	if !isModelAdmin && !api.isControllerAdmin {
		return params.ErrorResults{}, common.ErrPerm
	}

	branch, err := api.model.Branch(arg.BranchName)
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
func (api *API) CommitBranch(arg params.BranchArg) (params.IntResult, error) {
	result := params.IntResult{}

	isModelAdmin, err := api.hasAdminAccess()
	if err != nil {
		return result, errors.Trace(err)
	}
	if !isModelAdmin && !api.isControllerAdmin {
		return result, common.ErrPerm
	}

	generation, err := api.model.Branch(arg.BranchName)
	if err != nil {
		return result, errors.Trace(err)
	}

	if genId, err := generation.Commit(api.apiUser.Name()); err != nil {
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
func (api *API) BranchInfo(args params.BranchArgs) (params.GenerationResults, error) {
	result := params.GenerationResults{}

	isModelAdmin, err := api.hasAdminAccess()
	if err != nil {
		return result, errors.Trace(err)
	}
	if !isModelAdmin && !api.isControllerAdmin {
		return result, common.ErrPerm
	}

	// From clients, we expect a single branch name or none,
	// but we accommodate any number - they all must exist to avoid an error.
	// If no branch is supplied, get them all.
	// TODO (manadart 2019-04-11): Needs slight rework when model UUID
	// is removed from args for this API.
	var branches []Generation
	if len(args.Args) > 0 {
		branches := make([]Generation, len(args.Args))
		for i, arg := range args.Args {
			b, err := api.model.Branch(arg.BranchName)
			if err != nil {
				return generationInfoError(err)
			}
			branches[i] = b
		}
	} else {
		branches, err = api.model.Branches()
		if err != nil {
			return generationInfoError(err)
		}
	}

	results := make([]params.Generation, len(branches))
	for i, b := range branches {
		if results[i], err = api.oneBranchInfo(b); err != nil {
			return generationInfoError(err)
		}
	}
	result.Generations = results
	return result, nil
}

func (api *API) oneBranchInfo(branch Generation) (params.Generation, error) {
	delta := branch.Config()

	var apps []params.GenerationApplication
	for appName, units := range branch.AssignedUnits() {
		app, err := api.st.Application(appName)
		if err != nil {
			return params.Generation{}, errors.Trace(err)
		}

		// TODO (manadart 2019-02-22): As more aspects are made generational,
		// each should go into its own method - charm, resources etc.
		defaults, err := app.DefaultCharmConfig()
		if err != nil {
			return params.Generation{}, errors.Trace(err)
		}

		branchApp := params.GenerationApplication{
			ApplicationName: appName,
			Units:           units,
			ConfigChanges:   delta[appName].CurrentSettings(defaults),
		}
		apps = append(apps, branchApp)
	}

	return params.Generation{
		BranchName:   branch.BranchName(),
		Created:      branch.Created(),
		CreatedBy:    branch.CreatedBy(),
		Applications: apps,
	}, nil
}

// HasActiveBranch returns a true result if the input model has an "in-flight"
// branch matching the input name.
func (api *API) HasActiveBranch(arg params.BranchArg) (params.BoolResult, error) {
	result := params.BoolResult{}
	isModelAdmin, err := api.hasAdminAccess()
	if err != nil {
		return result, errors.Trace(err)
	}
	if !isModelAdmin && !api.isControllerAdmin {
		return result, common.ErrPerm
	}

	if _, err := api.model.Branch(arg.BranchName); err != nil {
		if errors.IsNotFound(err) {
			result.Result = false
		} else {
			result.Error = common.ServerError(err)
		}
	} else {
		result.Result = true
	}
	return result, nil
}

func generationInfoError(err error) (params.GenerationResults, error) {
	return params.GenerationResults{Error: common.ServerError(err)}, nil
}
