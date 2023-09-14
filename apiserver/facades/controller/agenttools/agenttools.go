// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package agenttools

import (
	"context"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/version/v2"

	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/environs/simplestreams"
	"github.com/juju/juju/environs/tools"
	coretools "github.com/juju/juju/internal/tools"
	"github.com/juju/juju/state"
)

var (
	findTools = tools.FindTools
)

// AgentToolsAPI implements the API used by the machine model worker.
type AgentToolsAPI struct {
	modelGetter ModelGetter
	newEnviron  newEnvironFunc
	authorizer  facade.Authorizer
	// tools lookup
	findTools        toolsFinder
	envVersionUpdate envVersionUpdater
	logger           loggo.Logger
}

// NewAgentToolsAPI creates a new instance of the Model API.
func NewAgentToolsAPI(
	modelGetter ModelGetter,
	newEnviron func() (environs.Environ, error),
	findTools toolsFinder,
	envVersionUpdate func(*state.Model, version.Number) error,
	authorizer facade.Authorizer,
	logger loggo.Logger,
) (*AgentToolsAPI, error) {
	return &AgentToolsAPI{
		modelGetter:      modelGetter,
		newEnviron:       newEnviron,
		authorizer:       authorizer,
		findTools:        findTools,
		envVersionUpdate: envVersionUpdate,
		logger:           logger,
	}, nil
}

// ModelGetter represents a struct that can provide a state.Model.
type ModelGetter interface {
	Model() (*state.Model, error)
}

type newEnvironFunc func() (environs.Environ, error)
type toolsFinder func(tools.SimplestreamsFetcher, environs.BootstrapEnviron, int, int, []string, coretools.Filter) (coretools.List, error)
type envVersionUpdater func(*state.Model, version.Number) error

func checkToolsAvailability(newEnviron newEnvironFunc, modelCfg *config.Config, finder toolsFinder) (version.Number, error) {
	currentVersion, ok := modelCfg.AgentVersion()
	if !ok || currentVersion == version.Zero {
		return version.Zero, nil
	}

	env, err := newEnviron()
	if err != nil {
		return version.Zero, errors.Annotatef(err, "cannot make model")
	}

	ss := simplestreams.NewSimpleStreams(simplestreams.DefaultDataSourceFactory())

	// finder receives major and minor as parameters as it uses them to filter versions and
	// only return patches for the passed major.minor (from major.minor.patch).
	// We'll try the released stream first, then fall back to the current configured stream
	// if no released tools are found.
	vers, err := finder(ss, env, currentVersion.Major, currentVersion.Minor, []string{tools.ReleasedStream}, coretools.Filter{})
	preferredStream := tools.PreferredStreams(&currentVersion, modelCfg.Development(), modelCfg.AgentStream())[0]
	if preferredStream != tools.ReleasedStream && errors.Cause(err) == coretools.ErrNoMatches {
		vers, err = finder(ss, env, currentVersion.Major, currentVersion.Minor, []string{preferredStream}, coretools.Filter{})
	}
	if err != nil {
		return version.Zero, errors.Annotatef(err, "cannot find available agent binaries")
	}
	// Newest also returns a list of the items in this list matching with the
	// newest version.
	newest, _ := vers.Newest()
	return newest, nil
}

var modelConfig = func(e *state.Model) (*config.Config, error) {
	return e.Config()
}

// Base implementation of envVersionUpdater
func envVersionUpdate(env *state.Model, ver version.Number) error {
	return env.UpdateLatestToolsVersion(ver)
}

func updateToolsAvailability(modelGetter ModelGetter, newEnviron newEnvironFunc, finder toolsFinder, update envVersionUpdater, logger loggo.Logger) error {
	model, err := modelGetter.Model()
	if err != nil {
		return errors.Annotate(err, "cannot get model")
	}
	cfg, err := modelConfig(model)
	if err != nil {
		return errors.Annotate(err, "cannot get config")
	}
	ver, err := checkToolsAvailability(newEnviron, cfg, finder)
	if err != nil {
		if errors.Is(err, errors.NotFound) {
			// No newer tools, so exit silently.
			return nil
		}
		return errors.Annotate(err, "cannot get latest version")
	}
	if ver == version.Zero {
		logger.Debugf("The lookup of agent binaries returned version Zero. This should only happen during bootstrap.")
		return nil
	}
	return update(model, ver)
}

// UpdateToolsAvailable invokes a lookup and further update in environ
// for new patches of the current tool versions.
func (api *AgentToolsAPI) UpdateToolsAvailable(ctx context.Context) error {
	if !api.authorizer.AuthController() {
		return apiservererrors.ErrPerm
	}
	return updateToolsAvailability(api.modelGetter, api.newEnviron, api.findTools, api.envVersionUpdate, api.logger)
}
