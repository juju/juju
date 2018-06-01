// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common

import (
	"github.com/juju/errors"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/constraints"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/context"
	"github.com/juju/juju/environs/instances"
)

func toParamsInstanceTypeResult(itypes []instances.InstanceType) []params.InstanceType {
	result := make([]params.InstanceType, len(itypes))
	for i, t := range itypes {
		virtType := ""
		if t.VirtType != nil {
			virtType = *t.VirtType
		}
		result[i] = params.InstanceType{
			Name:         t.Name,
			Arches:       t.Arches,
			CPUCores:     int(t.CpuCores),
			Memory:       int(t.Mem),
			RootDiskSize: int(t.RootDisk),
			VirtType:     virtType,
			Deprecated:   t.Deprecated,
			Cost:         int(t.Cost),
		}
	}
	return result
}

// NewInstanceTypeConstraints returns an instanceTypeConstraints with the passed
// parameters.
func NewInstanceTypeConstraints(env environs.Environ, ctx context.ProviderCallContext, constraints constraints.Value) instanceTypeConstraints {
	return instanceTypeConstraints{
		environ:     env,
		constraints: constraints,
		context:     ctx,
	}
}

// instanceTypeConstraints holds necessary params to filter instance types.
type instanceTypeConstraints struct {
	constraints constraints.Value
	environ     environs.Environ
	context     context.ProviderCallContext
}

// InstanceTypes returns a list of the available instance types in the provider according
// to the passed constraints.
func InstanceTypes(cons instanceTypeConstraints) (params.InstanceTypesResult, error) {
	instanceTypes, err := cons.environ.InstanceTypes(cons.context, cons.constraints)
	if err != nil {
		return params.InstanceTypesResult{}, errors.Trace(err)
	}

	return params.InstanceTypesResult{
		InstanceTypes: toParamsInstanceTypeResult(instanceTypes.InstanceTypes),
		CostUnit:      instanceTypes.CostUnit,
		CostCurrency:  instanceTypes.CostCurrency,
		CostDivisor:   instanceTypes.CostDivisor,
	}, nil
}
