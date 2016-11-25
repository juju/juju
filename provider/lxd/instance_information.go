// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lxd

import (
	"github.com/juju/errors"

	"github.com/juju/juju/constraints"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/instances"
)

var _ environs.InstanceTypesFetcher = (*environ)(nil)

// InstanceTypes implements InstanceTypesFetcher
func (env *environ) InstanceTypes(c constraints.Value) (instances.InstanceTypesWithCostMetadata, error) {
	return instances.InstanceTypesWithCostMetadata{}, errors.NotSupportedf("InstanceTypes")
}
