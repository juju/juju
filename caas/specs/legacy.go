// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package specs

import (
	// "github.com/juju/errors"

	apiextensionsv1beta1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1beta1"
)

// PodSpecLegacy defines the legacy version of data values used to configure
// a pod on the CAAS substrate.
type PodSpecLegacy struct {
	podSpec

	OmitServiceFrontend       bool                                                         `yaml:"omitServiceFrontend"`
	CustomResourceDefinitions map[string]apiextensionsv1beta1.CustomResourceDefinitionSpec `yaml:"-"`
}

// VersionLegacy defines the version number for pod spec version 0 - legacy.
const VersionLegacy Version = 0

// Validate returns an error if the spec is not valid.
func (spec *PodSpecLegacy) Validate() error {
	return spec.podSpec.Validate(VersionLegacy)

	// if err := spec.podSpec.Validate(); err != nil {
	// 	return errors.Trace(err)
	// }
	// return spec.validateVersion(VersionLegacy)
}
