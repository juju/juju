// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package assumes

import (
	"fmt"

	"github.com/juju/juju/core/semversion"
)

var (
	// A set of user-friendly descriptions for potentially supported
	// features that are known to the controller. This allows us to
	// generate better error messages when an "assumes" expression requests a
	// feature that is not included in the feature set supported by the
	// current model.
	UserFriendlyFeatureDescriptions = map[string]string{
		"juju":    "the version of Juju used by the model",
		"k8s-api": "the Kubernetes API lets charms query and manipulate the state of API objects in a Kubernetes cluster",
	}
)

// featureMissingErrs is a list of user-friendly error messages to return when
// a given feature is expected by a charm, but not present in the model.
var featureMissingErrs = map[string]string{
	"juju":    "charm requires Juju", // this should never happen
	"k8s-api": "charm must be deployed on a Kubernetes cloud",
}

// featureMissingErr returns a user-friendly error message to return when a
// given feature is expected by a charm, but not present in the model.
func featureMissingErr(featureName string) string {
	if errStr, ok := featureMissingErrs[featureName]; ok {
		return errStr
	}
	// We don't have a specific error message defined, so return a default.
	return fmt.Sprintf("charm requires feature %q but model does not support it", featureName)
}

// featureVersionMismatchErrs is a list of functions which generate
// user-friendly error messages when a given feature is present in the model,
// but the version is lower than required by the charm.
var featureVersionMismatchErrs = map[string]func(constraint, requiredVersion, actualVersion string) string{
	"juju": func(c, rv, av string) string {
		return fmt.Sprintf("charm requires Juju version %s %s, model has version %s", c, rv, av)
	},
	"k8s-api": func(c, rv, av string) string {
		return fmt.Sprintf("charm requires Kubernetes version %s %s, model is running on version %s", c, rv, av)
	},
}

// featureVersionMismatchErr returns a user-friendly error message to return when
// a given feature is present in the model, but the version is lower than
// required by the charm.
func featureVersionMismatchErr(featureName, constraint, requiredVersion, actualVersion string) string {
	if f, ok := featureVersionMismatchErrs[featureName]; ok {
		return f(constraint, requiredVersion, actualVersion)
	}
	// We don't have a specific error message defined, so return a default.
	return fmt.Sprintf("charm requires feature %q (version %s %s) but model currently supports version %s",
		featureName, constraint, requiredVersion, actualVersion)
}

// JujuFeature returns a new Feature representing the Juju API for the given
// version.
func JujuFeature(ver semversion.Number) Feature {
	return Feature{
		Name:        "juju",
		Description: UserFriendlyFeatureDescriptions["juju"],
		Version:     &ver,
	}
}

// K8sAPIFeature returns a new Feature representing the Kubernetes API for the
// given version.
func K8sAPIFeature(ver semversion.Number) Feature {
	return Feature{
		Name:        "k8s-api",
		Description: UserFriendlyFeatureDescriptions["k8s-api"],
		Version:     &ver,
	}
}
