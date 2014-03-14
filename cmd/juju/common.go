// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"launchpad.net/juju-core/cmd"
	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/environs/configstore"
	"launchpad.net/juju-core/errors"
)

// destroyPreparedEnviron destroys the environment and logs an error if it fails.
func destroyPreparedEnviron(env environs.Environ, store configstore.Storage, err *error, action string) {
	if *err == nil {
		return
	}
	if err := environs.Destroy(env, store); err != nil {
		logger.Errorf("%s failed, and the environment could not be destroyed: %v", action, err)
	}
}

// environFromName loads an existing environment or prepares a new one.
func environFromName(
	ctx *cmd.Context, envName string, resultErr *error, action string) (environs.Environ, func(), error) {

	store, err := configstore.Default()
	if err != nil {
		return nil, nil, err
	}
	var existing bool
	if environInfo, err := store.ReadInfo(envName); !errors.IsNotFoundError(err) {
		existing = true
		logger.Warningf("ignoring environments.yaml: using bootstrap config in %s", environInfo.Location())
	}
	environ, err := environs.PrepareFromName(envName, ctx, store)
	if err != nil {
		return nil, nil, err
	}
	cleanup := func() {
		if !existing {
			destroyPreparedEnviron(environ, store, resultErr, action)
		}
	}
	return environ, cleanup, nil
}
