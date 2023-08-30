// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package stateenvirons

import (
	"github.com/juju/errors"

	"github.com/juju/juju/caas"
	"github.com/juju/juju/core/assumes"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/state"
)

var (
	// Overridden by tests
	iaasEnvironGetter = GetNewEnvironFunc(environs.New)
	caasBrokerGetter  = GetNewCAASBrokerFunc(caas.New)
)

// SupportedFeatures returns the set of features that the model makes available
// for charms to use.
func SupportedFeatures(model Model, credentialService CredentialService) (assumes.FeatureSet, error) {
	var fs assumes.FeatureSet

	// Models always include a feature flag for the current Juju version
	modelConf, err := model.Config()
	if err != nil {
		return fs, errors.Annotate(err, "accessing model config")
	}

	agentVersion, _ := modelConf.AgentVersion()
	fs.Add(assumes.Feature{
		Name:        "juju",
		Description: assumes.UserFriendlyFeatureDescriptions["juju"],
		Version:     &agentVersion,
	})

	// Access the environment associated with the model and query any
	// substrate-specific features that might be available.
	var env interface{}
	switch model.Type() {
	case state.ModelTypeIAAS:
		iaasEnv, err := iaasEnvironGetter(model, credentialService)
		if err != nil {
			return fs, errors.Annotate(err, "accessing model environment")
		}
		env = iaasEnv
	case state.ModelTypeCAAS:
		caasEnv, err := caasBrokerGetter(model, credentialService)
		if err != nil {
			return fs, errors.Annotate(err, "accessing model environment")
		}
		env = caasEnv
	default:
		return fs, errors.NotSupportedf("model type %q", model.Type())
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
