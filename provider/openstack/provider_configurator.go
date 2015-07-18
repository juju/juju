// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Stub provider for OpenStack, using goose will be implemented here

package openstack

import (
	"gopkg.in/goose.v1/nova"
)

type OpenstackProviderConfigurator interface {
	UseSecurityGroups() bool
	InitialNetworks() []nova.ServerNetworks
	ModifyRunServerOptions(options *nova.RunServerOpts)
}

type defaultProviderConfigurator struct{}

func (c *defaultProviderConfigurator) UseSecurityGroups() bool {
	return true
}

func (c *defaultProviderConfigurator) InitialNetworks() []nova.ServerNetworks {
	return []nova.ServerNetworks{}
}

func (c *defaultProviderConfigurator) ModifyRunServerOptions(options *nova.RunServerOpts) {
}
