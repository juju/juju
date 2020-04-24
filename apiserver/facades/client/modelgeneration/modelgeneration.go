// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelgeneration

import (
	"fmt"

	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/names/v4"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/core/permission"
)

var logger = loggo.GetLogger("juju.apiserver.modelgeneration")

// API is the concrete implementation of the API endpoint.
type API struct {
	check             *common.BlockChecker
	authorizer        facade.Authorizer
	apiUser           names.UserTag
	isControllerAdmin bool
	st                State
	model             Model
	modelCache        ModelCache
}

type APIV3 struct {
	*API
}

type APIV2 struct {
	*APIV3
}

type APIV1 struct {
	*APIV2
}

// NewModelGenerationFacadeV4 provides the signature required for facade registration.
func NewModelGenerationFacadeV4(ctx facade.Context) (*API, error) {
	authorizer := ctx.Auth()
	st := &stateShim{State: ctx.State()}
	m, err := st.Model()
	if err != nil {
		return nil, errors.Trace(err)
	}
	mc, err := ctx.Controller().Model(st.ModelUUID())
	if err != nil {
		return nil, errors.Trace(err)
	}
	return NewModelGenerationAPI(st, authorizer, m, &modelCacheShim{Model: mc})
}

// NewModelGenerationFacadeV3 provides the signature required for facade registration.
func NewModelGenerationFacadeV3(ctx facade.Context) (*APIV3, error) {
	v4, err := NewModelGenerationFacadeV4(ctx)
	if err != nil {
		return nil, err
	}
	return &APIV3{v4}, nil

} // NewModelGenerationFacadeV2 provides the signature required for facade registration.
func NewModelGenerationFacadeV2(ctx facade.Context) (*APIV2, error) {
	v3, err := NewModelGenerationFacadeV3(ctx)
	if err != nil {
		return nil, err
	}
	return &APIV2{v3}, nil
}

// NewModelGenerationFacade provides the signature required for facade registration.
func NewModelGenerationFacade(ctx facade.Context) (*APIV1, error) {
	v2, err := NewModelGenerationFacadeV2(ctx)
	if err != nil {
		return nil, err
	}
	return &APIV1{v2}, nil
}

// NewModelGenerationAPI creates a new API endpoint for dealing with model generations.
func NewModelGenerationAPI(
	st State,
	authorizer facade.Authorizer,
	m Model,
	mc ModelCache,
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
		modelCache:        mc,
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
func (api *APIV2) TrackBranch(arg params.BranchTrackArg) (params.ErrorResults, error) {
	// For backwards compatibility, ensure we always set the NumUnits to 0
	arg.NumUnits = 0
	return api.API.TrackBranch(arg)
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
	// Ensure we guard against the numUnits being greater than 0 and the number
	// units/applications greater than 1. This is because we don't know how to
	// topographically distribute between all the applications and units,
	// especially if an error occurs whilst assigning the units.
	if arg.NumUnits > 0 && len(arg.Entities) > 1 {
		return params.ErrorResults{}, errors.Errorf("number of units and unit IDs can not be specified at the same time")
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
			result.Results[i].Error = common.ServerError(branch.AssignUnits(tag.Id(), arg.NumUnits))
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

	branch, err := api.model.Branch(arg.BranchName)
	if err != nil {
		return intResultsError(err)
	}

	if genId, err := branch.Commit(api.apiUser.Name()); err != nil {
		result.Error = common.ServerError(err)
	} else {
		result.Result = genId
	}
	return result, nil
}

// AbortBranch aborts the input branch, marking it complete.  However no
// changes are made applicable to the whole model.  No units may be assigned
// to the branch when aborting.
func (api *API) AbortBranch(arg params.BranchArg) (params.ErrorResult, error) {
	result := params.ErrorResult{}

	isModelAdmin, err := api.hasAdminAccess()
	if err != nil {
		return result, errors.Trace(err)
	}
	if !isModelAdmin && !api.isControllerAdmin {
		return result, common.ErrPerm
	}

	branch, err := api.model.Branch(arg.BranchName)
	if err != nil {
		result.Error = common.ServerError(err)
		return result, nil
	}

	if err := branch.Abort(api.apiUser.Name()); err != nil {
		result.Error = common.ServerError(err)
	}
	return result, nil
}

// BranchInfo will return details of branch identified by the input argument,
// including units on the branch and the configuration disjoint with the
// master generation.
// An error is returned if no in-flight branch matching in input is found.
func (api *API) BranchInfo(
	args params.BranchInfoArgs) (params.BranchResults, error) {
	result := params.BranchResults{}

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
	var branches []Generation
	if len(args.BranchNames) > 0 {
		branches = make([]Generation, len(args.BranchNames))
		for i, name := range args.BranchNames {
			if branches[i], err = api.model.Branch(name); err != nil {
				return branchResultsError(err)
			}
		}
	} else {
		if branches, err = api.model.Branches(); err != nil {
			return branchResultsError(err)
		}
	}

	results := make([]params.Generation, len(branches))
	for i, b := range branches {
		if results[i], err = api.oneBranchInfo(b, args.Detailed); err != nil {
			return branchResultsError(err)
		}
	}
	result.Generations = results
	return result, nil
}

// ShowCommit will return details a commit given by its generationId
// An error is returned if either no branch can be found corresponding to the generation id.
// Or the generation id given is below 1.
func (api *API) ShowCommit(arg params.GenerationId) (params.GenerationResult, error) {
	result := params.GenerationResult{}

	isModelAdmin, err := api.hasAdminAccess()
	if err != nil {
		return result, errors.Trace(err)
	}
	if !isModelAdmin && !api.isControllerAdmin {
		return result, common.ErrPerm
	}
	if arg.GenerationId < 1 {
		err := errors.Errorf("supplied generation id has to be higher than 0")
		return generationResultError(err)
	}

	branch, err := api.model.Generation(arg.GenerationId)
	if err != nil {
		result.Error = common.ServerError(err)
		return result, nil
	}

	generationCommit, err := api.getGenerationCommit(branch)
	if err != nil {
		return generationResultError(err)
	}

	result.Generation = generationCommit

	return result, nil
}

// ListCommits will return the commits, hence only branches with generation_id higher than 0
func (api *API) ListCommits() (params.BranchResults, error) {
	var result params.BranchResults

	isModelAdmin, err := api.hasAdminAccess()
	if err != nil {
		return result, errors.Trace(err)
	}
	if !isModelAdmin && !api.isControllerAdmin {
		return result, common.ErrPerm
	}

	var branches []Generation
	if branches, err = api.model.Generations(); err != nil {
		return branchResultsError(err)
	}

	results := make([]params.Generation, len(branches))
	for i, b := range branches {
		gen := params.Generation{
			BranchName:   b.BranchName(),
			Completed:    b.Completed(),
			CompletedBy:  b.CompletedBy(),
			GenerationId: b.GenerationId(),
		}
		results[i] = gen
	}

	result.Generations = results
	return result, nil
}

func (api *API) oneBranchInfo(branch Generation, detailed bool) (params.Generation, error) {
	deltas := branch.Config()

	var apps []params.GenerationApplication
	for appName, tracking := range branch.AssignedUnits() {
		app, err := api.st.Application(appName)
		if err != nil {
			return params.Generation{}, errors.Trace(err)
		}
		allUnits, err := app.UnitNames()
		if err != nil {
			return params.Generation{}, errors.Trace(err)
		}

		branchApp := params.GenerationApplication{
			ApplicationName: appName,
			UnitProgress:    fmt.Sprintf("%d/%d", len(tracking), len(allUnits)),
		}

		// Determine the effective charm configuration changes.
		defaults, err := app.DefaultCharmConfig()
		if err != nil {
			return params.Generation{}, errors.Trace(err)
		}
		branchApp.ConfigChanges = deltas[appName].EffectiveChanges(defaults)

		// TODO (manadart 2019-04-12): Charm URL.

		// TODO (manadart 2019-04-12): Resources.

		// Only include unit names if detailed info was requested.
		if detailed {
			trackingSet := set.NewStrings(tracking...)
			branchApp.UnitsTracking = trackingSet.SortedValues()
			branchApp.UnitsPending = set.NewStrings(allUnits...).Difference(trackingSet).SortedValues()
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

func (api *API) getGenerationCommit(branch Generation) (params.Generation, error) {
	generation, err := api.oneBranchInfo(branch, true)
	if err != nil {
		return params.Generation{}, errors.Trace(err)
	}
	return params.Generation{
		BranchName:   branch.BranchName(),
		Completed:    branch.Completed(),
		CompletedBy:  branch.CompletedBy(),
		GenerationId: branch.GenerationId(),
		Created:      branch.Created(),
		CreatedBy:    branch.CreatedBy(),
		Applications: generation.Applications,
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

	if _, err := api.modelCache.Branch(arg.BranchName); err != nil {
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

func branchResultsError(err error) (params.BranchResults, error) {
	return params.BranchResults{Error: common.ServerError(err)}, nil
}

func generationResultError(err error) (params.GenerationResult, error) {
	return params.GenerationResult{Error: common.ServerError(err)}, nil
}

func intResultsError(err error) (params.IntResult, error) {
	return params.IntResult{Error: common.ServerError(err)}, nil
}
