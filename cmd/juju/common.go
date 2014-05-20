// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"fmt"

	"github.com/juju/errors"

	"launchpad.net/juju-core/charm"
	"launchpad.net/juju-core/cmd"
	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/environs/config"
	"launchpad.net/juju-core/environs/configstore"
	"launchpad.net/juju-core/state/api"
)

// destroyPreparedEnviron destroys the environment and logs an error if it fails.
func destroyPreparedEnviron(ctx *cmd.Context, env environs.Environ, store configstore.Storage, err *error, action string) {
	if *err == nil {
		return
	}
	ctx.Infof("%s failed, destroying environment", action)
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
	if environInfo, err := store.ReadInfo(envName); !errors.IsNotFound(err) {
		existing = true
		logger.Warningf("ignoring environments.yaml: using bootstrap config in %s", environInfo.Location())
	}
	environ, err := environs.PrepareFromName(envName, ctx, store)
	if err != nil {
		return nil, nil, err
	}
	cleanup := func() {
		if !existing {
			destroyPreparedEnviron(ctx, environ, store, resultErr, action)
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
	// If series is not set, use configured default series
	if series == "" {
		if defaultSeries, ok := conf.DefaultSeries(); ok {
			series = defaultSeries
		}
	}
	// Otherwise, look up the best supported series for this charm
	if series == "" {
		if ref.Schema == "local" {
			possibleUrl := &charm.URL{Reference: ref, Series: "precise"}
			logger.Errorf(`The series is not specified in the environment (default-series) or with the charm. Did you mean:
	%s`, possibleUrl.String())
			return nil, fmt.Errorf("cannot resolve series for charm: %q", ref)
		}
		return client.ResolveCharm(ref)
	}
	return &charm.URL{Reference: ref, Series: series}, nil
}
