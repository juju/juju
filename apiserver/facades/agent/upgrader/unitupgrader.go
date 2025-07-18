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
	corebase "github.com/juju/juju/core/base"
	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/core/machine"
	"github.com/juju/juju/core/semversion"
	coreunit "github.com/juju/juju/core/unit"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	machineerrors "github.com/juju/juju/domain/machine/errors"
	"github.com/juju/juju/internal/errors"
	"github.com/juju/juju/internal/tools"
	"github.com/juju/juju/rpc/params"
)

// UnitUpgraderAPI provides access to the UnitUpgrader API facade.
type UnitUpgraderAPI struct {
	authorizer         facade.Authorizer
	modelAgentService  ModelAgentService
	applicationService ApplicationService
	watcherRegistry    facade.WatcherRegistry
}

// NewUnitUpgraderAPI creates a new server-side UnitUpgraderAPI facade.
func NewUnitUpgraderAPI(
	authorizer facade.Authorizer,
	modelAgentService ModelAgentService,
	applicationService ApplicationService,
	watcherRegistry facade.WatcherRegistry,
) *UnitUpgraderAPI {
	return &UnitUpgraderAPI{
		authorizer:         authorizer,
		modelAgentService:  modelAgentService,
		applicationService: applicationService,
		watcherRegistry:    watcherRegistry,
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
		unitName, err := coreunit.NewName(tag.Id())
		if err != nil {
			result.Results[i].Error = apiservererrors.ParamsErrorf(
				params.CodeTagInvalid,
				"invalid unit name %q",
				tag.Id(),
			)
			continue
		}
		unitAPIWatcher, err := u.modelAgentService.WatchUnitTargetAgentVersion(ctx, unitName)
		switch {
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
			result[i].Version, err = u.getMachineToolsVersion(ctx, tag)
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
		err = u.modelAgentService.SetUnitReportedAgentVersion(
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

	if err != nil {
		return err
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
			result.Results[i] = u.getMachineTools(ctx, tag)
		}
	}
	return result, nil
}

func (u *UnitUpgraderAPI) getAssignedMachine(ctx context.Context, tag names.Tag) (machine.Name, error) {
	// Check that we really have a unit tag.
	switch tag := tag.(type) {
	case names.UnitTag:
		unitName, err := coreunit.NewName(tag.Id())
		if err != nil {
			return "", apiservererrors.ErrPerm
		}
		machineName, err := u.applicationService.GetUnitMachineName(ctx, unitName)
		if errors.Is(err, applicationerrors.UnitNotFound) {
			return "", apiservererrors.ServerError(coreerrors.NotFound)
		} else if err != nil {
			return "", err
		}
		return machineName, nil
	default:
		return "", apiservererrors.ErrPerm
	}
}

func (u *UnitUpgraderAPI) getMachineTools(ctx context.Context, tag names.Tag) params.ToolsResult {
	var result params.ToolsResult
	machineName, err := u.getAssignedMachine(ctx, tag)
	if err != nil {
		result.Error = apiservererrors.ServerError(err)
		return result
	}

	machineTools, err := u.modelAgentService.GetMachineAgentBinaryMetadata(ctx, machineName)
	if errors.Is(err, machineerrors.MachineNotFound) {
		result.Error = apiservererrors.ServerError(coreerrors.NotFound)
		return result
	} else if err != nil {
		result.Error = apiservererrors.ServerError(err)
		return result
	}
	// We are okay returning the tools for just the one API server
	// address since the unit agent won't try to download tools that
	// are already present on the machine.
	result.ToolsList = tools.List{{
		Version: semversion.Binary{
			Number:  machineTools.Version.Number,
			Release: corebase.UbuntuOS,
			Arch:    machineTools.Version.Arch,
		},
		SHA256: machineTools.SHA256,
		Size:   machineTools.Size,
	}}
	return result
}

func (u *UnitUpgraderAPI) getMachineToolsVersion(ctx context.Context, tag names.Tag) (*semversion.Number, error) {
	machineName, err := u.getAssignedMachine(ctx, tag)
	if err != nil {
		return nil, err
	}
	machineTools, err := u.modelAgentService.GetMachineAgentBinaryMetadata(ctx, machineName)
	if errors.Is(err, machineerrors.MachineNotFound) {
		return nil, coreerrors.NotFound
	} else if err != nil {
		return nil, err
	}
	return &machineTools.Version.Number, nil
}
