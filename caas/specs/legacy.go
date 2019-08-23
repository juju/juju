// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package specs

// PodSpecLegacy defines the legacy version of data values used to configure
// a pod on the CAAS substrate.
type PodSpecLegacy struct {
	podSpec `yaml:",inline"`
}

// VersionLegacy defines the version number for pod spec version 0 - legacy.
const VersionLegacy Version = 0

// Validate returns an error if the spec is not valid.
func (spec *PodSpecLegacy) Validate() error {
	return spec.podSpec.Validate(VersionLegacy)
}
