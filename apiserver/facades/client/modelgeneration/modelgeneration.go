// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelgeneration

import (
	"context"
	"fmt"

	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"github.com/juju/names/v5"

	"github.com/juju/juju/apiserver/authentication"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/core/permission"
	"github.com/juju/juju/rpc/params"
)

// API is the concrete implementation of the API endpoint.
type API struct {
	authorizer facade.Authorizer
	apiUser    names.UserTag
	st         State
	model      Model
}

// NewModelGenerationAPI creates a new API endpoint for dealing with model generations.
func NewModelGenerationAPI(
	st State,
	authorizer facade.Authorizer,
	m Model,
) (*API, error) {
	if !authorizer.AuthClient() {
		return nil, apiservererrors.ErrPerm
	}
	// Since we know this is a user tag (because AuthClient is true),
	// we just do the type assertion to the UserTag.
	apiUser, _ := authorizer.GetAuthTag().(names.UserTag)

	return &API{
		authorizer: authorizer,
		apiUser:    apiUser,
		st:         st,
		model:      m,
	}, nil
}

func (api *API) hasAdminAccess(ctx context.Context) error {
	// We used to cache the result on the api object if the user was a superuser.
	// We don't do that anymore as permission caching could become invalid
	// for long lived connections.
	err := api.authorizer.HasPermission(usr, permission.SuperuserAccess, api.st.ControllerTag())
	if err != nil && !errors.Is(err, authentication.ErrorEntityMissingPermission) {
		return err
	}

	if err == nil {
		return nil
	}

	return api.authorizer.HasPermission(usr, permission.AdminAccess, api.model.ModelTag())
}

// AddBranch adds a new branch with the input name to the model.
func (api *API) AddBranch(ctx context.Context, arg params.BranchArg) (params.ErrorResult, error) {
	result := params.ErrorResult{}
	if err := api.hasAdminAccess(ctx); err != nil {
		return result, err
	}

	if err := model.ValidateBranchName(arg.BranchName); err != nil {
		result.Error = apiservererrors.ServerError(err)
	} else {
		result.Error = apiservererrors.ServerError(api.model.AddBranch(arg.BranchName, api.apiUser.Name()))
	}
	return result, nil
}

// TrackBranch marks the input units and/or applications as tracking the input
// branch, causing them to realise changes made under that branch.
func (api *API) TrackBranch(ctx context.Context, arg params.BranchTrackArg) (params.ErrorResults, error) {
	if err := api.hasAdminAccess(ctx); err != nil {
		return params.ErrorResults{}, err
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
			result.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}
		switch tag.Kind() {
		case names.ApplicationTagKind:
			result.Results[i].Error = apiservererrors.ServerError(branch.AssignUnits(tag.Id(), arg.NumUnits))
		case names.UnitTagKind:
			result.Results[i].Error = apiservererrors.ServerError(branch.AssignUnit(tag.Id()))
		default:
			result.Results[i].Error = apiservererrors.ServerError(
				errors.Errorf("expected names.UnitTag or names.ApplicationTag, got %T", tag))
		}
	}
	return result, nil
}

// CommitBranch commits the input branch, making its changes applicable to
// the whole model and marking it complete.
func (api *API) CommitBranch(ctx context.Context, arg params.BranchArg) (params.IntResult, error) {
	result := params.IntResult{}

	if err := api.hasAdminAccess(ctx); err != nil {
		return result, err
	}

	branch, err := api.model.Branch(arg.BranchName)
	if err != nil {
		return intResultsError(err)
	}

	if genId, err := branch.Commit(api.apiUser.Name()); err != nil {
		result.Error = apiservererrors.ServerError(err)
	} else {
		result.Result = genId
	}
	return result, nil
}

// AbortBranch aborts the input branch, marking it complete.  However no
// changes are made applicable to the whole model.  No units may be assigned
// to the branch when aborting.
func (api *API) AbortBranch(ctx context.Context, arg params.BranchArg) (params.ErrorResult, error) {
	result := params.ErrorResult{}

	if err := api.hasAdminAccess(ctx); err != nil {
		return result, err
	}

	branch, err := api.model.Branch(arg.BranchName)
	if err != nil {
		result.Error = apiservererrors.ServerError(err)
		return result, nil
	}

	if err := branch.Abort(api.apiUser.Name()); err != nil {
		result.Error = apiservererrors.ServerError(err)
	}
	return result, nil
}

// BranchInfo will return details of branch identified by the input argument,
// including units on the branch and the configuration disjoint with the
// master generation.
// An error is returned if no in-flight branch matching in input is found.
func (api *API) BranchInfo(
	ctx context.Context,
	args params.BranchInfoArgs) (params.BranchResults, error) {
	result := params.BranchResults{}

	if err := api.hasAdminAccess(ctx); err != nil {
		return result, err
	}

	// From clients, we expect a single branch name or none,
	// but we accommodate any number - they all must exist to avoid an error.
	// If no branch is supplied, get them all.
	var err error
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
func (api *API) ShowCommit(ctx context.Context, arg params.GenerationId) (params.GenerationResult, error) {
	result := params.GenerationResult{}

	if err := api.hasAdminAccess(ctx); err != nil {
		return result, err
	}

	if arg.GenerationId < 1 {
		err := errors.Errorf("supplied generation id has to be higher than 0")
		return generationResultError(err)
	}

	branch, err := api.model.Generation(arg.GenerationId)
	if err != nil {
		result.Error = apiservererrors.ServerError(err)
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
func (api *API) ListCommits(ctx context.Context) (params.BranchResults, error) {
	var result params.BranchResults

	if err := api.hasAdminAccess(ctx); err != nil {
		return result, err
	}

	var err error
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
func (api *API) HasActiveBranch(ctx context.Context, arg params.BranchArg) (params.BoolResult, error) {
	result := params.BoolResult{}
	if err := api.hasAdminAccess(ctx); err != nil {
		return result, err
	}

	if _, err := api.model.Branch(arg.BranchName); err != nil {
		if errors.Is(err, errors.NotFound) {
			result.Result = false
		} else {
			result.Error = apiservererrors.ServerError(err)
		}
	} else {
		result.Result = true
	}
	return result, nil
}

func branchResultsError(err error) (params.BranchResults, error) {
	return params.BranchResults{Error: apiservererrors.ServerError(err)}, nil
}

func generationResultError(err error) (params.GenerationResult, error) {
	return params.GenerationResult{Error: apiservererrors.ServerError(err)}, nil
}

func intResultsError(err error) (params.IntResult, error) {
	return params.IntResult{Error: apiservererrors.ServerError(err)}, nil
}
