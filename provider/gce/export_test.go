// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package gce

import (
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/context"
	"github.com/juju/juju/environs/imagemetadata"
	"github.com/juju/juju/environs/instances"
	"github.com/juju/juju/provider/gce/google"
)

var (
	Provider                 environs.EnvironProvider = providerInstance
	NewInstance                                       = newInstance
	CheckInstanceType                                 = checkInstanceType
	GetMetadata                                       = getMetadata
	GetDisks                                          = getDisks
	UbuntuImageBasePath                               = ubuntuImageBasePath
	UbuntuDailyImageBasePath                          = ubuntuDailyImageBasePath
	WindowsImageBasePath                              = windowsImageBasePath
)

func ExposeInstBase(inst instances.Instance) *google.Instance {
	return inst.(*environInstance).base
}

func ExposeInstEnv(inst *environInstance) *environ {
	return inst.env
}

func ExposeEnvConfig(env *environ) *environConfig {
	return env.ecfg
}

func ExposeEnvConnection(env *environ) gceConnection {
	return env.gce
}

func GlobalFirewallName(env *environ) string {
	return env.globalFirewallName()
}

func ParsePlacement(env *environ, ctx context.ProviderCallContext, placement string) (*instPlacement, error) {
	return env.parsePlacement(ctx, placement)
}

func FinishInstanceConfig(env *environ, args environs.StartInstanceParams, spec *instances.InstanceSpec) error {
	return env.finishInstanceConfig(args, spec)
}

func FindInstanceSpec(
	env *environ,
	ic *instances.InstanceConstraint,
	imageMetadata []*imagemetadata.ImageMetadata,
) (*instances.InstanceSpec, error) {
	return env.findInstanceSpec(ic, imageMetadata)
}

func BuildInstanceSpec(env *environ, args environs.StartInstanceParams) (*instances.InstanceSpec, error) {
	return env.buildInstanceSpec(args)
}

func NewRawInstance(env *environ, ctx context.ProviderCallContext, args environs.StartInstanceParams, spec *instances.InstanceSpec) (*google.Instance, error) {
	return env.newRawInstance(ctx, args, spec)
}

func GetHardwareCharacteristics(env *environ, spec *instances.InstanceSpec, inst *environInstance) *instance.HardwareCharacteristics {
	return env.getHardwareCharacteristics(spec, inst)
}

func GetInstances(env *environ, ctx context.ProviderCallContext) ([]instances.Instance, error) {
	return env.instances(ctx)
}
