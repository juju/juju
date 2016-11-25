// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package rackspace

import (
	"github.com/juju/juju/environs"
	"github.com/juju/juju/provider/openstack"
)

func NewProvider(innerProvider environs.EnvironProvider) environs.EnvironProvider {
	return &environProvider{innerProvider}
}

func NewEnviron(innerEnviron environs.Environ) environs.Environ {
	return environ{innerEnviron}
}

func OpenstackProvider(p environs.EnvironProvider) *openstack.EnvironProvider {
	return p.(*environProvider).EnvironProvider.(*openstack.EnvironProvider)
}

var Bootstrap = &bootstrap

var WaitSSH = &waitSSH

var NewInstanceConfigurator = &newInstanceConfigurator
