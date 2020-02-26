// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package specs

import (
	"github.com/juju/errors"
)

// PodSpecLegacy defines the legacy version of data values used to configure
// a pod on the CAAS substrate.
type PodSpecLegacy struct {
	podSpecBaseV2 `yaml:",inline"`
}

// VersionLegacy defines the version number for pod spec version 0 - legacy.
const VersionLegacy Version = 0

// Validate returns an error if the spec is not valid.
func (spec *PodSpecLegacy) Validate() error {
	return errors.Trace(spec.podSpecBaseV2.Validate(VersionLegacy))
}
