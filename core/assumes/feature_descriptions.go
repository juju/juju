// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package assumes

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
