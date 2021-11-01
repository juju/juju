// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package stateenvirons

import (
	"github.com/juju/errors"
	"github.com/juju/juju/core/assumes"
	"github.com/juju/juju/environs"
)

var (
	// Overridden by tests
	environGetter = environs.GetEnviron
)

// SupportedFeatures returns the set of features that the model makes available
// for charms to use.
func SupportedFeatures(model Model, newEnviron environs.NewEnvironFunc) (assumes.FeatureSet, error) {
	var fs assumes.FeatureSet

	// Models always include a feature flag for the current Juju version
	modelConf, err := model.Config()
	if err != nil {
		return fs, errors.Annotate(err, "accessing model config")
	}

	agentVersion, _ := modelConf.AgentVersion()
	fs.Add(assumes.Feature{
		Name:        "juju",
		Description: "the version of Juju used by the model",
		Version:     &agentVersion,
	})

	// Access the environment associated with the model and query any
	// substrate-specific features that might be available.
	env, err := environGetter(EnvironConfigGetter{Model: model}, newEnviron)
	if err != nil {
		return fs, errors.Annotate(err, "accessing model environment")
	}

	if featureEnumerator, supported := env.(environs.SupportedFeatureEnumerator); supported {
		envFs, err := featureEnumerator.SupportedFeatures()
		if err != nil {
			return fs, errors.Annotate(err, "enumerating features supported by environment")
		}

		fs.Merge(envFs)
	}

	return fs, nil
}
