// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package gce

import (
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/instances"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/provider/gce/google"
	"github.com/juju/juju/storage"
)

var (
	Provider          environs.EnvironProvider = providerInstance
	NewInstance                                = newInstance
	CheckInstanceType                          = checkInstanceType
	GetMetadata                                = getMetadata
	GetDisks                                   = getDisks
	ConfigImmutable                            = configImmutableFields
)

func ExposeInstBase(inst *environInstance) *google.Instance {
	return inst.base
}

func ExposeInstEnv(inst *environInstance) *environ {
	return inst.env
}

func ParseAvailabilityZones(env *environ, args environs.StartInstanceParams) ([]string, error) {
	return env.parseAvailabilityZones(args)
}

func UnsetEnvConfig(env *environ) {
	env.ecfg = nil
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

func ParsePlacement(env *environ, placement string) (*instPlacement, error) {
	return env.parsePlacement(placement)
}

func FinishInstanceConfig(env *environ, args environs.StartInstanceParams, spec *instances.InstanceSpec) error {
	return env.finishInstanceConfig(args, spec)
}

func FindInstanceSpec(env *environ, stream string, ic *instances.InstanceConstraint) (*instances.InstanceSpec, error) {
	return env.findInstanceSpec(stream, ic)
}

func BuildInstanceSpec(env *environ, args environs.StartInstanceParams) (*instances.InstanceSpec, error) {
	return env.buildInstanceSpec(args)
}

func NewRawInstance(env *environ, args environs.StartInstanceParams, spec *instances.InstanceSpec) (*google.Instance, error) {
	return env.newRawInstance(args, spec)
}

func GetHardwareCharacteristics(env *environ, spec *instances.InstanceSpec, inst *environInstance) *instance.HardwareCharacteristics {
	return env.getHardwareCharacteristics(spec, inst)
}

func GetInstances(env *environ) ([]instance.Instance, error) {
	return env.instances()
}

// Storage
func GCEStorageProvider() storage.Provider {
	return &storageProvider{}
}
