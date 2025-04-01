// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgrader

import (
	"context"
	"fmt"

	"github.com/juju/names/v6"

	"github.com/juju/juju/apiserver/common"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/apiserver/internal"
	"github.com/juju/juju/controller"
	coreagentbinary "github.com/juju/juju/core/agentbinary"
	coreerrors "github.com/juju/juju/core/errors"
	corelogger "github.com/juju/juju/core/logger"
	coremachine "github.com/juju/juju/core/machine"
	coreunit "github.com/juju/juju/core/unit"
	jujuversion "github.com/juju/juju/core/version"
	"github.com/juju/juju/core/watcher"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	controllernodeerrors "github.com/juju/juju/domain/controllernode/errors"
	machineerrors "github.com/juju/juju/domain/machine/errors"
	modelerrors "github.com/juju/juju/domain/model/errors"
	"github.com/juju/juju/internal/errors"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
)

// ControllerConfigGetter defines a method for getting the controller config.
type ControllerConfigGetter interface {
	ControllerConfig(context.Context) (controller.Config, error)
}

type Upgrader interface {
	WatchAPIVersion(ctx context.Context, args params.Entities) (params.NotifyWatchResults, error)
	DesiredVersion(ctx context.Context, args params.Entities) (params.VersionResults, error)
	Tools(ctx context.Context, args params.Entities) (params.ToolsResults, error)
	SetTools(ctx context.Context, args params.EntitiesVersion) (params.ErrorResults, error)
}

// UpgraderAPI provides access to the Upgrader API facade.
type UpgraderAPI struct {
	*common.ToolsGetter

	st              *state.State
	authorizer      facade.Authorizer
	logger          corelogger.Logger
	watcherRegistry facade.WatcherRegistry

	controllerNodeService ControllerNodeService
	modelAgentService     ModelAgentService
}

// NewUpgraderAPI creates a new server-side UpgraderAPI facade.
func NewUpgraderAPI(
	toolsGetter *common.ToolsGetter,
	st *state.State,
	authorizer facade.Authorizer,
	logger corelogger.Logger,
	watcherRegistry facade.WatcherRegistry,
	controllerNodeService ControllerNodeService,
	modelAgentService ModelAgentService,
) *UpgraderAPI {
	return &UpgraderAPI{
		ToolsGetter:           toolsGetter,
		st:                    st,
		authorizer:            authorizer,
		logger:                logger,
		watcherRegistry:       watcherRegistry,
		controllerNodeService: controllerNodeService,
		modelAgentService:     modelAgentService,
	}
}

// WatchAPIVersion starts a watcher to track if there is a new version
// of the API that we want to upgrade an application, machine, model or
// unit to.
func (u *UpgraderAPI) WatchAPIVersion(ctx context.Context, args params.Entities) (params.NotifyWatchResults, error) {
	result := params.NotifyWatchResults{
		Results: make([]params.NotifyWatchResult, len(args.Entities)),
	}
	for i, agent := range args.Entities {
		tag, err := names.ParseTag(agent.Tag)
		if err != nil {
			result.Results[i].Error = apiservererrors.ParamsErrorf(
				params.CodeTagInvalid, "%s", err,
			)
			continue
		}

		tagID := tag.Id()
		var upgraderAPIWatcher watcher.NotifyWatcher

		switch tag.Kind() {
		case names.ControllerTagKind, names.ModelTagKind:
			upgraderAPIWatcher, err = u.modelAgentService.WatchModelTargetAgentVersion(ctx)
		case names.MachineTagKind:
			upgraderAPIWatcher, err = u.modelAgentService.WatchMachineTargetAgentVersion(ctx, coremachine.Name(tagID))
		case names.UnitTagKind:
			// Used in kubernetes models.
			upgraderAPIWatcher, err = u.modelAgentService.WatchUnitTargetAgentVersion(ctx, tagID)
		default:
			result.Results[i].Error = apiservererrors.ParamsErrorf(
				params.CodeNotValid, "%s", tag.String(),
			)
			continue
		}

		switch {
		case errors.Is(err, modelerrors.NotFound), errors.Is(err, applicationerrors.UnitNotFound),
			errors.Is(err, machineerrors.MachineNotFound):
			result.Results[i].Error = apiservererrors.ParamsErrorf(
				params.CodeNotFound, "%s", tag,
			)
			continue
		case err != nil:
			// We don't understand this error. At this stage we consider it an
			// internal server error and bail out of the call completely.
			return params.NotifyWatchResults{}, fmt.Errorf("cannot watch api version for tag %q: %w", tag, err)
		}

		result.Results[i].NotifyWatcherId, _, err = internal.EnsureRegisterWatcher[struct{}](
			ctx, u.watcherRegistry, upgraderAPIWatcher,
		)
		if err != nil {
			return params.NotifyWatchResults{}, fmt.Errorf(
				"registering %s api version watcher: %w",
				tag, err,
			)
		}
	}
	return result, nil
}

type hasIsManager interface {
	IsManager() bool
}

func (u *UpgraderAPI) entityIsManager(tag names.Tag) bool {
	entity, err := u.st.FindEntity(tag)
	if err != nil {
		return false
	}
	if m, ok := entity.(hasIsManager); !ok {
		return false
	} else {
		return m.IsManager()
	}
}

// DesiredVersion reports the Agent Version that we want that agent to be running
func (u *UpgraderAPI) DesiredVersion(ctx context.Context, args params.Entities) (params.VersionResults, error) {
	results := make([]params.VersionResult, len(args.Entities))
	if len(args.Entities) == 0 {
		return params.VersionResults{}, nil
	}
	agentVersion, err := u.modelAgentService.GetModelTargetAgentVersion(ctx)
	if err != nil {
		return params.VersionResults{}, apiservererrors.ServerError(err)
	}
	// Is the desired version greater than the current API server version?
	isNewerVersion := agentVersion.Compare(jujuversion.Current) > 0
	for i, entity := range args.Entities {
		tag, err := names.ParseTag(entity.Tag)
		if err != nil {
			results[i].Error = apiservererrors.ServerError(err)
			continue
		}
		err = apiservererrors.ErrPerm
		if u.authorizer.AuthOwner(tag) {
			// Only return the globally desired agent version if the
			// asking entity is a machine agent with JobManageModel or
			// if this API server is running the globally desired agent
			// version. Otherwise report this API server's current
			// agent version.
			//
			// This ensures that state machine agents will upgrade
			// first - once they have restarted and are running the
			// new version other agents will start to see the new
			// agent version.
			if !isNewerVersion || u.entityIsManager(tag) {
				results[i].Version = &agentVersion
			} else {
				u.logger.Debugf(context.TODO(), "desired version is %s, but current version is %s and agent is not a manager node", agentVersion, jujuversion.Current)
				results[i].Version = &jujuversion.Current
			}
			err = nil
		}
		results[i].Error = apiservererrors.ServerError(err)
	}
	return params.VersionResults{Results: results}, nil
}

// SetTools is responsible for updating a a set of entities reported agent
// version.
func (u *UpgraderAPI) SetTools(
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
func (u *UpgraderAPI) setEntityToolVersion(
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
	case names.ControllerAgentTagKind:
		err = u.controllerNodeService.SetControllerNodeReportedAgentVersion(
			ctx,
			tag.Id(),
			reportedVersion,
		)
	case names.MachineTagKind:
		err = u.modelAgentService.SetMachineReportedAgentVersion(
			ctx,
			coremachine.Name(tag.Id()),
			reportedVersion,
		)
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
	case errors.Is(err, machineerrors.MachineNotFound):
		return errors.Errorf(
			"machine for tag %q not found", tag,
		).Add(coreerrors.NotFound)
	case errors.Is(err, applicationerrors.UnitNotFound):
		return errors.Errorf(
			"unit for tag %q not found", tag,
		).Add(coreerrors.NotFound)
	case errors.Is(err, controllernodeerrors.NotFound):
		return errors.Errorf(
			"controller for tag %q not found", tag,
		).Add(coreerrors.NotFound)
	}

	if err != nil {
		return err
	}
	return nil
}
