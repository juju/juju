// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package ec2

import (
	"github.com/juju/errors"

	"github.com/juju/juju/constraints"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/instances"
)

var _ environs.InstanceTypesFetcher = (*environ)(nil)

// InstanceTypes implements InstanceTypesFetcher
func (e *environ) InstanceTypes(c constraints.Value) (instances.InstanceTypesWithCostMetadata, error) {
	iTypes, err := e.supportedInstanceTypes()
	if err != nil {
		return instances.InstanceTypesWithCostMetadata{}, errors.Trace(err)
	}
	iTypes, err = instances.MatchingInstanceTypes(iTypes, "", c)
	if err != nil {
		return instances.InstanceTypesWithCostMetadata{}, errors.Trace(err)
	}
	return instances.InstanceTypesWithCostMetadata{
		InstanceTypes: iTypes,
		CostUnit:      "$USD/hour",
		CostDivisor:   1000,
		CostCurrency:  "USD"}, nil
}
