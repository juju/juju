// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package agenttools

import (
	"context"

	"github.com/juju/errors"

	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
	corelogger "github.com/juju/juju/core/logger"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/simplestreams"
	"github.com/juju/juju/environs/tools"
	coretools "github.com/juju/juju/internal/tools"
	"github.com/juju/juju/internal/version"
	"github.com/juju/juju/state"
)

// AgentToolsAPI implements the API used by the machine model worker.
type AgentToolsAPI struct {
	modelGetter ModelGetter
	newEnviron  newEnvironFunc
	authorizer  facade.Authorizer
	// tools lookup
	findTools        toolsFinder
	envVersionUpdate envVersionUpdater
	logger           corelogger.Logger

	modelConfigService ModelConfigService
	modelAgentService  ModelAgentService
}

// NewAgentToolsAPI creates a new instance of the Model API.
func NewAgentToolsAPI(
	modelGetter ModelGetter,
	newEnviron func() (environs.Environ, error),
	findTools toolsFinder,
	envVersionUpdate func(*state.Model, version.Number) error,
	authorizer facade.Authorizer,
	logger corelogger.Logger,
	modelConfigService ModelConfigService,
	modelAgentService ModelAgentService,
) (*AgentToolsAPI, error) {
	return &AgentToolsAPI{
		modelGetter:        modelGetter,
		newEnviron:         newEnviron,
		authorizer:         authorizer,
		findTools:          findTools,
		envVersionUpdate:   envVersionUpdate,
		logger:             logger,
		modelConfigService: modelConfigService,
		modelAgentService:  modelAgentService,
	}, nil
}

// ModelGetter represents a struct that can provide a state.Model.
type ModelGetter interface {
	Model() (*state.Model, error)
}

type newEnvironFunc func() (environs.Environ, error)
type toolsFinder func(context.Context, tools.SimplestreamsFetcher, environs.BootstrapEnviron, int, int, []string, coretools.Filter) (coretools.List, error)
type envVersionUpdater func(*state.Model, version.Number) error

func (api *AgentToolsAPI) checkToolsAvailability(ctx context.Context) (version.Number, error) {
	currentVersion, err := api.modelAgentService.GetModelTargetAgentVersion(ctx)
	if err != nil {
		return version.Zero, errors.Annotate(err, "getting agent version from service")
	}

	env, err := api.newEnviron()
	if err != nil {
		return version.Zero, errors.Annotatef(err, "cannot make cloud provider")
	}

	ss := simplestreams.NewSimpleStreams(simplestreams.DefaultDataSourceFactory())
	modelCfg, err := api.modelConfigService.ModelConfig(ctx)
	if err != nil {
		return version.Zero, errors.Annotate(err, "cannot get model config")
	}

	preferredStreams := tools.PreferredStreams(&currentVersion, modelCfg.Development(), modelCfg.AgentStream())
	vers, err := api.findTools(ctx, ss, env, currentVersion.Major, currentVersion.Minor, preferredStreams, coretools.Filter{})
	if err != nil {
		return version.Zero, errors.Annotatef(err, "cannot find available agent binaries")
	}
	// Newest also returns a list of the items in this list matching with the
	// newest version.
	newest, _ := vers.Newest()
	return newest, nil
}

// Base implementation of envVersionUpdater
func envVersionUpdate(env *state.Model, ver version.Number) error {
	return env.UpdateLatestToolsVersion(ver.String())
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
	if ver == version.Zero {
		api.logger.Debugf(context.TODO(), "The lookup of agent binaries returned version Zero. This should only happen during bootstrap.")
		return nil
	}

	model, err := api.modelGetter.Model()
	if err != nil {
		return errors.Annotate(err, "cannot get model")
	}
	return api.envVersionUpdate(model, ver)
}

// UpdateToolsAvailable invokes a lookup and further update in environ
// for new patches of the current tool versions.
func (api *AgentToolsAPI) UpdateToolsAvailable(ctx context.Context) error {
	if !api.authorizer.AuthController() {
		return apiservererrors.ErrPerm
	}
	return api.updateToolsAvailability(ctx)
}
