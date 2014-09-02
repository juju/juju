// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"fmt"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	"gopkg.in/juju/charm.v3"

	"github.com/juju/juju/api"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/environs/configstore"
)

// destroyPreparedEnviron destroys the environment and logs an error if it fails.
func destroyPreparedEnviron(
	ctx *cmd.Context,
	env environs.Environ,
	store configstore.Storage,
	action string,
) {
	ctx.Infof("%s failed, destroying environment", action)
	if err := environs.Destroy(env, store); err != nil {
		logger.Errorf("%s failed, and the environment could not be destroyed: %v", action, err)
	}
}

// environFromName loads an existing environment or prepares a new
// one. If there are no errors, it returns the environ and a closure to
// clean up in case we need to further up the stack. If an error has
// occurred, the environment and cleanup function will be nil, and the
// error will be filled in.
func environFromName(
	ctx *cmd.Context,
	envName string,
	action string,
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

	if env, err = environs.PrepareFromName(envName, ctx, store); err != nil {
		return nil, nil, err
	}

	cleanup = func() {
		if !envExisted {
			destroyPreparedEnviron(ctx, env, store, action)
		}
	}

	return env, cleanup, nil
}

// resolveCharmURL returns a resolved charm URL, given a charm location string.
// If the series is not resolved, the environment default-series is used, or if
// not set, the series is resolved with the state server.
func resolveCharmURL(url string, client *api.Client, conf *config.Config) (*charm.URL, error) {
	ref, err := charm.ParseReference(url)
	if err != nil {
		return nil, err
	}
	// If series is not set, use configured default series
	if ref.Series == "" {
		if defaultSeries, ok := conf.DefaultSeries(); ok {
			ref.Series = defaultSeries
		}
	}
	if ref.Series != "" {
		return ref.URL("")
	}
	// Otherwise, look up the best supported series for this charm
	if ref.Schema != "local" {
		return client.ResolveCharm(ref)
	}
	possibleURL := *ref
	possibleURL.Series = "precise"
	logger.Errorf("The series is not specified in the environment (default-series) or with the charm. Did you mean:\n\t%s", &possibleURL)
	return nil, fmt.Errorf("cannot resolve series for charm: %q", ref)
}
