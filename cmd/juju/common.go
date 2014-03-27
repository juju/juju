// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"launchpad.net/juju-core/charm"
	"launchpad.net/juju-core/cmd"
	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/environs/config"
	"launchpad.net/juju-core/environs/configstore"
	"launchpad.net/juju-core/errors"
	"launchpad.net/juju-core/state/api"
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

// resolveCharmURL returns a resolved charm URL, given a charm location string.
// If the series is not resolved, the environment default-series is used, or if
// not set, the series is resolved with the state server.
func resolveCharmURL(url string, client *api.Client, conf *config.Config) (*charm.URL, error) {
	ref, series, err := charm.ParseReference(url)
	if err != nil {
		return nil, err
	}
	if series == "" {
		series = conf.DefaultSeries()
	}
	if series == "" {
		return client.ResolveCharm(ref)
	}
	return &charm.URL{Reference: ref, Series: series}, nil
}

// resolveCharmURL1dot16 returns a resolved charm URL for older state servers
// that do not support ResolveCharm. The default series "precise" is
// appropriate for these environments.
func resolveCharmURL1dot16(url string, conf *config.Config) (*charm.URL, error) {
	ref, series, err := charm.ParseReference(url)
	if err != nil {
		return nil, err
	}

	if series == "" {
		series = conf.DefaultSeries()
	}
	if series == "" {
		logger.Warningf(`ResolveCharm not supported by the API server, falling back to default series "precise".`)
		series = "precise"
	}
	return &charm.URL{Reference: ref, Series: series}, err
}
