// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package openstack

import (
	"github.com/juju/errors"

	"github.com/juju/juju/v3/core/constraints"
	"github.com/juju/juju/v3/environs"
	"github.com/juju/juju/v3/environs/context"
	"github.com/juju/juju/v3/environs/instances"
)

var _ environs.InstanceTypesFetcher = (*Environ)(nil)

func (e *Environ) InstanceTypes(ctx context.ProviderCallContext, c constraints.Value) (instances.InstanceTypesWithCostMetadata, error) {
	result := instances.InstanceTypesWithCostMetadata{}
	return result, errors.NotSupportedf("InstanceTypes")
}
