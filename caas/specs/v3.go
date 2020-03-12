// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package specs

import (
	"github.com/juju/errors"
)

// PodSpecV3 defines the data values used to configure
// a pod on the CAAS substrate for version 3.
type PodSpecV3 struct {
	podSpecBase    `json:",inline" yaml:",inline"`
	ServiceAccount *PrimeServiceAccountSpecV3 `json:"serviceAccount,omitempty" yaml:"serviceAccount,omitempty"`
}

// Version3 defines the version number for pod spec version 3.
const Version3 Version = 3

// Validate returns an error if the spec is not valid.
func (spec *PodSpecV3) Validate() error {
	if err := spec.podSpecBase.Validate(Version3); err != nil {
		return errors.Trace(err)
	}
	if spec.ServiceAccount != nil {
		// TODO: do we want to restrict the prime sa can only have 1 role/clusterrole???????
		return errors.Trace(spec.ServiceAccount.Validate())
	}
	return nil
}
