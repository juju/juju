// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package gce

import (
	"github.com/juju/juju/environs"
	"github.com/juju/juju/provider/gce/google"
)

var (
	Provider    environs.EnvironProvider = providerInstance
	NewInstance                          = newInstance
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
