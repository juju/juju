// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package agenttools

import (
	"context"

	"github.com/juju/errors"
	"github.com/juju/version/v2"

	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
	corelogger "github.com/juju/juju/core/logger"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/environs/simplestreams"
	"github.com/juju/juju/environs/tools"
	coretools "github.com/juju/juju/internal/tools"
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
	currentVersion, err := api.modelAgentService.GetModelAgentVersion(ctx)
	if err != nil {
		return version.Zero, errors.Annotate(err, "getting agent version from service")
	}
	if currentVersion == version.Zero {
		return version.Zero, nil
	}

	env, err := api.newEnviron()
	if err != nil {
		return version.Zero, errors.Annotatef(err, "cannot make model")
	}

	ss := simplestreams.NewSimpleStreams(simplestreams.DefaultDataSourceFactory())

	// finder receives major and minor as parameters as it uses them to filter versions and
	// only return patches for the passed major.minor (from major.minor.patch).
	// We'll try the released stream first, then fall back to the current configured stream
	// if no released tools are found.
	vers, err := api.findTools(ctx, ss, env, currentVersion.Major, currentVersion.Minor, []string{tools.ReleasedStream}, coretools.Filter{})
	if errors.Is(err, coretools.ErrNoMatches) {
		var modelCfg *config.Config
		modelCfg, err = api.modelConfigService.ModelConfig(ctx)
		if err != nil {
			return version.Zero, errors.Annotate(err, "cannot get config")
		}

		preferredStream := tools.PreferredStreams(&currentVersion, modelCfg.Development(), modelCfg.AgentStream())[0]
		if preferredStream != tools.ReleasedStream {
			vers, err = api.findTools(ctx, ss, env, currentVersion.Major, currentVersion.Minor, []string{preferredStream}, coretools.Filter{})
		}
	}
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
	return env.UpdateLatestToolsVersion(ver)
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
		api.logger.Debugf("The lookup of agent binaries returned version Zero. This should only happen during bootstrap.")
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
