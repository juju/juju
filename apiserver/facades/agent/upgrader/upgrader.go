// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgrader

import (
	"context"
	"fmt"

	"github.com/juju/errors"
	"github.com/juju/names/v5"

	"github.com/juju/juju/apiserver/common"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/apiserver/internal"
	"github.com/juju/juju/controller"
	corelogger "github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/machine"
	"github.com/juju/juju/core/objectstore"
	jujuversion "github.com/juju/juju/core/version"
	"github.com/juju/juju/core/watcher"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	machineerrors "github.com/juju/juju/domain/machine/errors"
	modelerrors "github.com/juju/juju/domain/model/errors"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/stateenvirons"
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
	*common.ToolsSetter

	st                *state.State
	m                 *state.Model
	authorizer        facade.Authorizer
	logger            corelogger.Logger
	modelAgentService ModelAgentService
	watcherRegistry   facade.WatcherRegistry
}

// NewUpgraderAPI creates a new server-side UpgraderAPI facade.
func NewUpgraderAPI(
	controllerConfigGetter ControllerConfigGetter,
	ctrlSt *state.State,
	st *state.State,
	authorizer facade.Authorizer,
	logger corelogger.Logger,
	cloudService common.CloudService,
	credentialService common.CredentialService,
	modelConfigService common.ModelConfigService,
	modelAgentService ModelAgentService,
	controllerStore objectstore.ObjectStore,
	watcherRegistry facade.WatcherRegistry,
) (*UpgraderAPI, error) {
	if !authorizer.AuthMachineAgent() && !authorizer.AuthApplicationAgent() && !authorizer.AuthModelAgent() && !authorizer.AuthUnitAgent() {
		return nil, apiservererrors.ErrPerm
	}
	getCanReadWrite := func() (common.AuthFunc, error) {
		return authorizer.AuthOwner, nil
	}
	model, err := st.Model()
	if err != nil {
		return nil, err
	}
	urlGetter := common.NewToolsURLGetter(model.UUID(), ctrlSt)
	configGetter := stateenvirons.EnvironConfigGetter{
		Model: model, ModelConfigService: modelConfigService, CloudService: cloudService, CredentialService: credentialService}
	newEnviron := common.EnvironFuncForModel(model, cloudService, credentialService, configGetter)
	toolsFinder := common.NewToolsFinder(controllerConfigGetter, st, urlGetter, newEnviron, controllerStore)
	return &UpgraderAPI{
		ToolsGetter:       common.NewToolsGetter(st, modelAgentService, st, urlGetter, toolsFinder, getCanReadWrite),
		ToolsSetter:       common.NewToolsSetter(st, getCanReadWrite),
		st:                st,
		m:                 model,
		authorizer:        authorizer,
		logger:            logger,
		modelAgentService: modelAgentService,
		watcherRegistry:   watcherRegistry,
	}, nil
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
		var errMessage string
		var upgraderAPIWatcher watcher.NotifyWatcher

		switch tag.Kind() {
		case names.ControllerTagKind, names.ModelTagKind:
			errMessage = fmt.Sprintf("model %q", tagID)
			upgraderAPIWatcher, err = u.modelAgentService.WatchModelTargetAgentVersion(ctx)
		case names.MachineTagKind:
			errMessage = fmt.Sprintf("machine %q", tagID)
			upgraderAPIWatcher, err = u.modelAgentService.WatchMachineTargetAgentVersion(ctx, machine.Name(tagID))
		case names.UnitTagKind:
			// Used in kubernetes models.
			errMessage = fmt.Sprintf("unit %q", tagID)
			upgraderAPIWatcher, err = u.modelAgentService.WatchUnitTargetAgentVersion(ctx, tagID)
		default:
			result.Results[i].Error = apiservererrors.ParamsErrorf(
				params.CodeNotValid, "%s", tag.String(),
			)
			continue
		}

		switch {
		case errors.Is(err, modelerrors.NotFound), errors.Is(err, applicationerrors.ApplicationNotFound),
			errors.Is(err, machineerrors.MachineNotFound):
			result.Results[i].Error = apiservererrors.ParamsErrorf(
				params.CodeNotFound, "%s", errMessage,
			)
			continue
		case err != nil:
			// We don't understand this error. At this stage we consider it an
			// internal server error and bail out of the call completely.
			return params.NotifyWatchResults{}, fmt.Errorf(
				"cannot watch api version for %s: %w",
				errMessage, err,
			)
		}

		result.Results[i].NotifyWatcherId, _, err = internal.EnsureRegisterWatcher[struct{}](
			ctx, u.watcherRegistry, upgraderAPIWatcher,
		)
		if err != nil {
			return params.NotifyWatchResults{}, fmt.Errorf(
				"registering %s api version watcher: %w",
				errMessage, err,
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
				u.logger.Debugf("desired version is %s, but current version is %s and agent is not a manager node", agentVersion, jujuversion.Current)
				results[i].Version = &jujuversion.Current
			}
			err = nil
		}
		results[i].Error = apiservererrors.ServerError(err)
	}
	return params.VersionResults{Results: results}, nil
}
