// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// +build go1.3

package lxd

import (
	"github.com/juju/juju/environs"
	"github.com/juju/juju/tools/lxdclient"
)

var (
	Provider           environs.EnvironProvider = providerInstance
	GlobalFirewallName                          = (*environ).globalFirewallName
	NewInstance                                 = newInstance
)

func ExposeInstRaw(inst *environInstance) *lxdclient.Instance {
	return inst.raw
}

func ExposeInstEnv(inst *environInstance) *environ {
	return inst.env
}

func UnsetEnvConfig(env *environ) {
	env.ecfg = nil
}

func ExposeEnvConfig(env *environ) *environConfig {
	return env.ecfg
}

func ExposeEnvClient(env *environ) lxdInstances {
	return env.raw.lxdInstances
}
