// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package machinemanager

import (
	"github.com/juju/errors"

	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/context"
	"github.com/juju/juju/environs/instances"
	"github.com/juju/juju/rpc/params"
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
			CPUCores:     int(t.CpuCores),
			Memory:       int(t.Mem),
			RootDiskSize: int(t.RootDisk),
			VirtType:     virtType,
			Cost:         int(t.Cost),
		}
		if t.Arch != "" {
			result[i].Arches = []string{t.Arch}
		}
	}
	return result
}

// newInstanceTypeConstraints returns an instanceTypeConstraints with the passed
// parameters.
func newInstanceTypeConstraints(env environs.Environ, constraints constraints.Value) instanceTypeConstraints {
	return instanceTypeConstraints{
		environ:     env,
		constraints: constraints,
	}
}

// instanceTypeConstraints holds necessary params to filter instance types.
type instanceTypeConstraints struct {
	constraints constraints.Value
	environ     environs.Environ
}

// getInstanceTypes returns a list of the available instance types in the provider according
// to the passed constraints.
func getInstanceTypes(ctx context.ProviderCallContext, cons instanceTypeConstraints) (params.InstanceTypesResult, error) {
	instanceTypes, err := cons.environ.InstanceTypes(ctx, cons.constraints)
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
