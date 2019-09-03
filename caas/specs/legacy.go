// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package specs

import (
	"github.com/juju/errors"
)

// PodSpecLegacy defines the legacy version of data values used to configure
// a pod on the CAAS substrate.
type PodSpecLegacy struct {
	podSpecBase `yaml:",inline"`

	// legacy version has containers/initContainers two blocks.
	InitContainers []ContainerSpec `json:"initContainers" yaml:"initContainers"`
}

// VersionLegacy defines the version number for pod spec version 0 - legacy.
const VersionLegacy Version = 0

// Validate returns an error if the spec is not valid.
func (spec *PodSpecLegacy) Validate() error {
	for i, c := range spec.InitContainers {
		// set init to true.
		c.Init = true
		if err := c.Validate(); err != nil {
			return errors.Trace(err)
		}
		// ensure init set to true for init containers.
		spec.InitContainers[i] = c
	}
	return spec.podSpecBase.Validate(VersionLegacy)
}
