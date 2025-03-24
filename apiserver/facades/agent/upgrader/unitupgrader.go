// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgrader

import (
	"context"
	"fmt"

	"github.com/juju/names/v6"

	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/apiserver/internal"
	coreagentbinary "github.com/juju/juju/core/agentbinary"
	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/core/semversion"
	coreunit "github.com/juju/juju/core/unit"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	"github.com/juju/juju/internal/errors"
	"github.com/juju/juju/internal/tools"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
)

// UnitUpgraderAPI provides access to the UnitUpgrader API facade.
type UnitUpgraderAPI struct {
	st                *state.State
	authorizer        facade.Authorizer
	modelAgentService ModelAgentService
	watcherRegistry   facade.WatcherRegistry
	unitService       UnitService
}

// NewUnitUpgraderAPI creates a new server-side UnitUpgraderAPI facade.
func NewUnitUpgraderAPI(
	st *state.State,
	authorizer facade.Authorizer,
	modelAgentService ModelAgentService,
	watcherRegistry facade.WatcherRegistry,
	unitService UnitService,
) *UnitUpgraderAPI {
	return &UnitUpgraderAPI{
		st:                st,
		authorizer:        authorizer,
		modelAgentService: modelAgentService,
		watcherRegistry:   watcherRegistry,
		unitService:       unitService,
	}
}

// WatchAPIVersion starts a watcher to track if there is a new version
// of the API that we want to upgrade the unit agent to.
func (u *UnitUpgraderAPI) WatchAPIVersion(ctx context.Context, args params.Entities) (params.NotifyWatchResults, error) {
	result := params.NotifyWatchResults{
		Results: make([]params.NotifyWatchResult, len(args.Entities)),
	}
	for i, agent := range args.Entities {
		tag, err := names.ParseUnitTag(agent.Tag)
		if err != nil {
			result.Results[i].Error = apiservererrors.ServerError(apiservererrors.ErrPerm)
			continue
		}
		unitName := tag.Id()
		unitAPIWatcher, err := u.modelAgentService.WatchUnitTargetAgentVersion(ctx, unitName)
		switch {
		case errors.Is(err, coreerrors.NotValid):
			result.Results[i].Error = apiservererrors.ParamsErrorf(
				params.CodeTagInvalid,
				"invalid unit name %q",
				unitName,
			)
			continue
		case errors.Is(err, applicationerrors.UnitNotFound):
			result.Results[i].Error = apiservererrors.ParamsErrorf(
				params.CodeNotFound,
				"unit %q does not exist",
				unitName,
			)
		case err != nil:
			// We don't understand this error. At this stage we consider it an
			// internal server error and bail out of the call completely.
			return params.NotifyWatchResults{}, fmt.Errorf(
				"cannot watch api version for unit %q: %w",
				unitName, err,
			)
		}

		result.Results[i].NotifyWatcherId, _, err = internal.EnsureRegisterWatcher[struct{}](
			ctx, u.watcherRegistry, unitAPIWatcher,
		)
		if err != nil {
			return params.NotifyWatchResults{}, fmt.Errorf(
				"registering unit %q api version watcher: %w",
				unitName, err,
			)
		}
	}
	return result, nil
}

// DesiredVersion reports the Agent Version that we want that unit to be running.
// The desired version is what the unit's assigned machine is running.
func (u *UnitUpgraderAPI) DesiredVersion(ctx context.Context, args params.Entities) (params.VersionResults, error) {
	result := make([]params.VersionResult, len(args.Entities))
	for i, entity := range args.Entities {
		tag, err := names.ParseTag(entity.Tag)
		if err != nil {
			result[i].Error = apiservererrors.ServerError(apiservererrors.ErrPerm)
			continue
		}
		err = apiservererrors.ErrPerm
		if u.authorizer.AuthOwner(tag) {
			result[i].Version, err = u.getMachineToolsVersion(tag)
		}
		result[i].Error = apiservererrors.ServerError(err)
	}
	return params.VersionResults{Results: result}, nil
}

// SetTools is responsible for updating a a set of entities reported agent
// version.
func (u *UnitUpgraderAPI) SetTools(
	ctx context.Context,
	args params.EntitiesVersion,
) (params.ErrorResults, error) {
	results := params.ErrorResults{
		Results: make([]params.ErrorResult, len(args.AgentTools)),
	}

	for i, entityVersion := range args.AgentTools {
		tag, err := names.ParseTag(entityVersion.Tag)
		if err != nil {
			results.Results[i].Error = apiservererrors.ServerError(apiservererrors.ErrPerm)
			continue
		}

		if !u.authorizer.AuthOwner(tag) {
			results.Results[i].Error = apiservererrors.ServerError(apiservererrors.ErrPerm)
			continue
		}

		err = u.setEntityToolVersion(ctx, tag, entityVersion)
		results.Results[i].Error = apiservererrors.ServerError(err)
	}

	return results, nil
}

// setEntityToolVersion is responsible for taking a Juju entity identified by
// tag and setting it's reported agent version in the relevant domain.
func (u *UnitUpgraderAPI) setEntityToolVersion(
	ctx context.Context,
	tag names.Tag,
	arg params.EntityVersion,
) error {
	reportedVersion := coreagentbinary.Version{
		Number: arg.Tools.Version.Number,
		Arch:   arg.Tools.Version.Arch,
	}

	var err error
	switch tag.Kind() {
	case names.UnitTagKind:
		err = u.unitService.SetReportedUnitAgentVersion(
			ctx,
			coreunit.Name(tag.Id()),
			reportedVersion,
		)
	default:
		return apiservererrors.NotSupportedError(tag, "agent binaries")
	}

	switch {
	case errors.Is(err, coreerrors.NotValid):
		return errors.Errorf(
			"agent version %q supplied is not valid for tag %q",
			arg.Tools.Version, tag,
		).Add(coreerrors.NotValid)
	case errors.Is(err, coreerrors.NotSupported):
		return errors.Errorf(
			"architecture %q not support for tag %q",
			arg.Tools.Version.Arch, tag,
		).Add(coreerrors.NotSupported)
	case errors.Is(err, applicationerrors.UnitNotFound):
		return errors.Errorf(
			"unit for tag %q not found", tag,
		).Add(coreerrors.NotFound)
	}

	return nil
}

// Tools finds the tools necessary for the given agents.
func (u *UnitUpgraderAPI) Tools(ctx context.Context, args params.Entities) (params.ToolsResults, error) {
	result := params.ToolsResults{
		Results: make([]params.ToolsResult, len(args.Entities)),
	}
	for i, entity := range args.Entities {
		result.Results[i].Error = apiservererrors.ServerError(apiservererrors.ErrPerm)
		tag, err := names.ParseTag(entity.Tag)
		if err != nil {
			continue
		}
		if u.authorizer.AuthOwner(tag) {
			result.Results[i] = u.getMachineTools(tag)
		}
	}
	return result, nil
}

func (u *UnitUpgraderAPI) getAssignedMachine(tag names.Tag) (*state.Machine, error) {
	// Check that we really have a unit tag.
	switch tag := tag.(type) {
	case names.UnitTag:
		unit, err := u.st.Unit(tag.Id())
		if err != nil {
			return nil, apiservererrors.ErrPerm
		}
		id, err := unit.AssignedMachineId()
		if err != nil {
			return nil, err
		}
		return u.st.Machine(id)
	default:
		return nil, apiservererrors.ErrPerm
	}
}

func (u *UnitUpgraderAPI) getMachineTools(tag names.Tag) params.ToolsResult {
	var result params.ToolsResult
	machine, err := u.getAssignedMachine(tag)
	if err != nil {
		result.Error = apiservererrors.ServerError(err)
		return result
	}
	machineTools, err := machine.AgentTools()
	if err != nil {
		result.Error = apiservererrors.ServerError(err)
		return result
	}
	// We are okay returning the tools for just the one API server
	// address since the unit agent won't try to download tools that
	// are already present on the machine.
	result.ToolsList = tools.List{machineTools}
	return result
}

func (u *UnitUpgraderAPI) getMachineToolsVersion(tag names.Tag) (*semversion.Number, error) {
	machine, err := u.getAssignedMachine(tag)
	if err != nil {
		return nil, err
	}
	machineTools, err := machine.AgentTools()
	if err != nil {
		return nil, err
	}
	return &machineTools.Version.Number, nil
}
