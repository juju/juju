// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package specs

import (
// "github.com/juju/errors"
)

// PodSpecV2 defines the data values used to configure
// a pod on the CAAS substrate for version 2.
type PodSpecV2 struct {
	// TODO: should be V1 but not V2 ???????????
	podSpec
}

// Version2 defines the version number for pod spec version 2.
const Version2 Version = 2

// Validate returns an error if the spec is not valid.
func (spec *PodSpecV2) Validate() error {
	return spec.podSpec.Validate(Version2)

	// if err := spec.podSpec.Validate(); err != nil {
	// 	return errors.Trace(err)
	// }
	// return spec.validateVersion(Version2)
}
