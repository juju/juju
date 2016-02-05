// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package agenttools

import (
	"github.com/juju/errors"
	"github.com/juju/loggo"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/environs/tools"
	"github.com/juju/juju/state"
	coretools "github.com/juju/juju/tools"
	"github.com/juju/juju/version"
)

func init() {
	common.RegisterStandardFacade("AgentTools", 1, NewAgentToolsAPI)
}

var logger = loggo.GetLogger("juju.apiserver.model")

var (
	findTools = tools.FindTools
)

// AgentToolsAPI implements the API used by the machine model worker.
type AgentToolsAPI struct {
	st         EnvironGetter
	authorizer common.Authorizer
	// tools lookup
	findTools        toolsFinder
	envVersionUpdate envVersionUpdater
}

// NewAgentToolsAPI creates a new instance of the Model API.
func NewAgentToolsAPI(st *state.State, resources *common.Resources, authorizer common.Authorizer) (*AgentToolsAPI, error) {
	return &AgentToolsAPI{
		st:               st,
		authorizer:       authorizer,
		findTools:        findTools,
		envVersionUpdate: envVersionUpdate,
	}, nil
}

// EnvironGetter represents a struct that can provide a state.Environment.
type EnvironGetter interface {
	Model() (*state.Model, error)
}

type toolsFinder func(environs.Environ, int, int, string, coretools.Filter) (coretools.List, error)
type envVersionUpdater func(*state.Model, version.Number) error

var newEnvirons = environs.New

func checkToolsAvailability(cfg *config.Config, finder toolsFinder) (version.Number, error) {
	currentVersion, ok := cfg.AgentVersion()
	if !ok || currentVersion == version.Zero {
		return version.Zero, nil
	}

	env, err := newEnvirons(cfg)
	if err != nil {
		return version.Zero, errors.Annotatef(err, "cannot make model")
	}

	// finder receives major and minor as parameters as it uses them to filter versions and
	// only return patches for the passed major.minor (from major.minor.patch).
	// We'll try the released stream first, then fall back to the current configured stream
	// if no released tools are found.
	vers, err := finder(env, currentVersion.Major, currentVersion.Minor, tools.ReleasedStream, coretools.Filter{})
	preferredStream := tools.PreferredStream(&currentVersion, cfg.Development(), cfg.AgentStream())
	if preferredStream != tools.ReleasedStream && errors.Cause(err) == coretools.ErrNoMatches {
		vers, err = finder(env, currentVersion.Major, currentVersion.Minor, preferredStream, coretools.Filter{})
	}
	if err != nil {
		return version.Zero, errors.Annotatef(err, "cannot find available tools")
	}
	// Newest also returns a list of the items in this list matching with the
	// newest version.
	newest, _ := vers.Newest()
	return newest, nil
}

var envConfig = func(e *state.Model) (*config.Config, error) {
	return e.Config()
}

// Base implementation of envVersionUpdater
func envVersionUpdate(env *state.Model, ver version.Number) error {
	return env.UpdateLatestToolsVersion(ver)
}

func updateToolsAvailability(st EnvironGetter, finder toolsFinder, update envVersionUpdater) error {
	env, err := st.Model()
	if err != nil {
		return errors.Annotate(err, "cannot get model")
	}
	cfg, err := envConfig(env)
	if err != nil {
		return errors.Annotate(err, "cannot get config")
	}
	ver, err := checkToolsAvailability(cfg, finder)
	if err != nil {
		if errors.IsNotFound(err) {
			// No newer tools, so exit silently.
			return nil
		}
		return errors.Annotate(err, "cannot get latest version")
	}
	if ver == version.Zero {
		logger.Debugf("tools lookup returned version Zero, this should only happen during bootstrap.")
		return nil
	}
	return update(env, ver)
}

// UpdateToolsAvailable invokes a lookup and further update in environ
// for new patches of the current tool versions.
func (api *AgentToolsAPI) UpdateToolsAvailable() error {
	if !api.authorizer.AuthModelManager() {
		return common.ErrPerm
	}
	return updateToolsAvailability(api.st, api.findTools, api.envVersionUpdate)
}
