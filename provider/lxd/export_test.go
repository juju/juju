// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lxd

import (
	"github.com/juju/juju/container/lxd"
	"github.com/juju/juju/tools/lxdclient"
)

var (
	NewInstance = newInstance
)

func ExposeInstRaw(inst *environInstance) *lxdclient.Instance {
	return inst.raw
}

func ExposeInstEnv(inst *environInstance) *environ {
	return inst.env
}

func ExposeEnvConfig(env *environ) *environConfig {
	return env.ecfg
}

func ExposeEnvClient(env *environ) lxdInstances {
	return env.raw.lxdInstances
}

func GetImageSources(env *environ) ([]lxd.RemoteServer, error) {
	return env.getImageSources()
}
