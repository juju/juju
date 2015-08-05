// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package newtoolsversionchecker

import (
	"github.com/juju/errors"

	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/environs/tools"
	"github.com/juju/juju/state"
	coretools "github.com/juju/juju/tools"
	"github.com/juju/juju/version"
)

var (
	findTools = tools.FindTools
)

type toolFinder func(environs.Environ, int, int, coretools.Filter) (coretools.List, error)
type envVersionUpdater func(*state.Environment, version.Number) error

var newEnvirons = environs.New

func checkToolsAvailability(cfg *config.Config, find toolFinder) (version.Number, error) {
	currentVersion, ok := cfg.AgentVersion()
	if !ok || currentVersion == version.Zero {
		return version.Zero, nil
	}

	env, err := newEnvirons(cfg)
	if err != nil {
		return version.Zero, errors.Annotatef(err, "cannot make environ")
	}

	vers, err := find(env, currentVersion.Major, currentVersion.Minor, coretools.Filter{})
	if err != nil {
		return version.Zero, errors.Annotatef(err, "canot find available tools")
	}
	newest, _ := vers.Newest()
	return newest, nil
}

var envConfig = func(e *state.Environment) (*config.Config, error) {
	return e.Config()
}

func envVersionUpdate(env *state.Environment, ver version.Number) error {
	return env.UpdateLatestToolsVersion(ver.String())
}

func updateToolsAvailability(st EnvironmentCapable, find toolFinder, update envVersionUpdater) error {
	env, err := st.Environment()
	if err != nil {
		return errors.Annotate(err, "cannot get environment")
	}
	cfg, err := envConfig(env)
	if err != nil {
		return errors.Annotate(err, "cannot get config")
	}
	ver, err := checkToolsAvailability(cfg, find)
	if err != nil {
		return errors.Annotate(err, "cannot get latest version")
	}
	logger.Debugf("found new tools %q", ver)
	return update(env, ver)
}
