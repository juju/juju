// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package commands

import (
	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/loggo"

	"github.com/juju/juju/cmd/envcmd"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/configstore"
)

// destroyPreparedEnviron destroys the environment and logs an error
// if it fails.
var destroyPreparedEnviron = destroyPreparedEnvironProductionFunc

var logger = loggo.GetLogger("juju.cmd.juju")

func destroyPreparedEnvironProductionFunc(
	ctx *cmd.Context,
	env environs.Environ,
	store configstore.Storage,
	action string,
) {
	ctx.Infof("%s failed, destroying model", action)
	if err := environs.Destroy(env, store); err != nil {
		logger.Errorf("the model could not be destroyed: %v", err)
	}
}

var destroyEnvInfo = destroyEnvInfoProductionFunc

func destroyEnvInfoProductionFunc(
	ctx *cmd.Context,
	cfgName string,
	store configstore.Storage,
	action string,
) {
	ctx.Infof("%s failed, cleaning up the model.", action)
	if err := environs.DestroyInfo(cfgName, store); err != nil {
		logger.Errorf("the model jenv file could not be cleaned up: %v", err)
	}
}

// environFromName loads an existing environment or prepares a new
// one. If there are no errors, it returns the environ and a closure to
// clean up in case we need to further up the stack. If an error has
// occurred, the environment and cleanup function will be nil, and the
// error will be filled in.
var environFromName = environFromNameProductionFunc

func environFromNameProductionFunc(
	ctx *cmd.Context,
	envName string,
	action string,
	ensureNotBootstrapped func(environs.Environ) error,
) (env environs.Environ, cleanup func(), err error) {

	store, err := configstore.Default()
	if err != nil {
		return nil, nil, err
	}

	envExisted := false
	if environInfo, err := store.ReadInfo(envName); err == nil {
		envExisted = true
		logger.Warningf(
			"ignoring environments.yaml: using bootstrap config in %s",
			environInfo.Location(),
		)
	} else if !errors.IsNotFound(err) {
		return nil, nil, err
	}

	cleanup = func() {
		// Distinguish b/t removing the jenv file or tearing down the
		// environment. We want to remove the jenv file if preparation
		// was not successful. We want to tear down the environment
		// only in the case where the environment didn't already
		// exist.
		if env == nil {
			logger.Debugf("Destroying model info.")
			destroyEnvInfo(ctx, envName, store, action)
		} else if !envExisted && ensureNotBootstrapped(env) != environs.ErrAlreadyBootstrapped {
			logger.Debugf("Destroying model.")
			destroyPreparedEnviron(ctx, env, store, action)
		}
	}

	if env, err = environs.PrepareFromName(envName, envcmd.BootstrapContext(ctx), store); err != nil {
		return nil, cleanup, err
	}

	return env, cleanup, err
}
