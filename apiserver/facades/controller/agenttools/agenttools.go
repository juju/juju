// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package agenttools

import (
	"context"

	"github.com/juju/errors"

	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
	corelogger "github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/semversion"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/simplestreams"
	"github.com/juju/juju/environs/tools"
	coretools "github.com/juju/juju/internal/tools"
)

// AgentToolsAPI implements the API used by the machine model worker.
type AgentToolsAPI struct {
	authorizer facade.Authorizer
	// tools lookup
	findTools toolsFinder
	logger    corelogger.Logger

	modelConfigService ModelConfigService
	modelAgentService  ModelAgentService
	machineService     MachineService
}

// NewAgentToolsAPI creates a new instance of the Model API.
func NewAgentToolsAPI(
	findTools toolsFinder,
	authorizer facade.Authorizer,
	logger corelogger.Logger,
	machineService MachineService,
	modelConfigService ModelConfigService,
	modelAgentService ModelAgentService,
) (*AgentToolsAPI, error) {
	return &AgentToolsAPI{
		authorizer:         authorizer,
		findTools:          findTools,
		logger:             logger,
		machineService:     machineService,
		modelConfigService: modelConfigService,
		modelAgentService:  modelAgentService,
	}, nil
}

type toolsFinder func(context.Context, tools.SimplestreamsFetcher, environs.BootstrapEnviron, int, int, []string, coretools.Filter) (coretools.List, error)

func (api *AgentToolsAPI) checkToolsAvailability(ctx context.Context) (semversion.Number, error) {
	currentVersion, err := api.modelAgentService.GetModelTargetAgentVersion(ctx)
	if err != nil {
		return semversion.Zero, errors.Annotate(err, "getting agent version from service")
	}

	ss := simplestreams.NewSimpleStreams(simplestreams.DefaultDataSourceFactory())
	modelCfg, err := api.modelConfigService.ModelConfig(ctx)
	if err != nil {
		return semversion.Zero, errors.Annotate(err, "cannot get model config")
	}

	env, err := api.machineService.GetBootstrapEnviron(ctx)
	if err != nil {
		return semversion.Zero, errors.Annotatef(err, "cannot get cloud provider")
	}

	preferredStreams := tools.PreferredStreams(&currentVersion, modelCfg.Development(), modelCfg.AgentStream())
	vers, err := api.findTools(ctx, ss, env, currentVersion.Major, currentVersion.Minor, preferredStreams, coretools.Filter{})
	if err != nil {
		return semversion.Zero, errors.Annotatef(err, "cannot find available agent binaries")
	}
	// Newest also returns a list of the items in this list matching with the
	// newest version.
	newest, _ := vers.Newest()
	return newest, nil
}

func (api *AgentToolsAPI) updateToolsAvailability(ctx context.Context) error {
	ver, err := api.checkToolsAvailability(ctx)
	if err != nil {
		if errors.Is(err, errors.NotFound) {
			// No newer tools, so exit silently.
			return nil
		}
		return errors.Annotate(err, "cannot get latest version")
	}
	if ver == semversion.Zero {
		api.logger.Debugf(ctx, "The lookup of agent binaries returned version Zero. This should only happen during bootstrap.")
		return nil
	}

	err = api.modelAgentService.UpdateLatestAgentVersion(ctx, ver)
	if err != nil {
		return errors.Annotate(err, "updating latest agent version")
	}
	return nil
}

// UpdateToolsAvailable invokes a lookup and further update in environ
// for new patches of the current tool versions.
func (api *AgentToolsAPI) UpdateToolsAvailable(ctx context.Context) error {
	if !api.authorizer.AuthController() {
		return apiservererrors.ErrPerm
	}
	return api.updateToolsAvailability(ctx)
}
