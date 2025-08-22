// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package gce

import (
	"google.golang.org/api/compute/v1"

	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/context"
	"github.com/juju/juju/environs/imagemetadata"
	"github.com/juju/juju/environs/instances"
)

var (
	Provider             environs.EnvironProvider = providerInstance
	GetMetadata                                   = getMetadata
	GetDisks                                      = getDisks
	Bootstrap                                     = &bootstrap
	FirewallerSuffixFunc                          = &randomSuffixNamer
)

func GlobalFirewallName(env *environ) string {
	return env.globalFirewallName()
}

func ParsePlacement(env *environ, ctx context.ProviderCallContext, placement string) (*compute.Zone, error) {
	return env.parsePlacement(ctx, placement)
}

func FinishInstanceConfig(env *environ, args environs.StartInstanceParams, spec *instances.InstanceSpec) error {
	return env.finishInstanceConfig(args, spec)
}

func FindInstanceSpec(
	env *environ,
	ic *instances.InstanceConstraint,
	imageMetadata []*imagemetadata.ImageMetadata,
	instanceTypes []instances.InstanceType,
) (*instances.InstanceSpec, error) {
	return env.findInstanceSpec(ic, imageMetadata, instanceTypes)
}

func BuildInstanceSpec(env *environ, ctx context.ProviderCallContext, args environs.StartInstanceParams) (*instances.InstanceSpec, error) {
	return env.buildInstanceSpec(ctx, args)
}

func GetHardwareCharacteristics(env *environ, spec *instances.InstanceSpec, inst *environInstance) *instance.HardwareCharacteristics {
	return env.getHardwareCharacteristics(spec, inst)
}
