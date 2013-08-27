// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provider

import (
	"launchpad.net/juju-core/environs/config"
	"launchpad.net/juju-core/instance"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/api"
)

func GetDNSNames(instances []instance.Instance) []string {
	return getDNSNames(instances)
}

func GetStateInfo(cfg *config.Config, hostnames []string) (*state.Info, *api.Info) {
	return getStateInfo(cfg, hostnames)
}

func ComposeAddresses(hostnames []string, port int) []string {
	return composeAddresses(hostnames, port)
}
