// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Stub provider for OpenStack, using goose will be implemented here

package openstack

import (
	"gopkg.in/goose.v1/nova"

	"github.com/juju/juju/cloudconfig/cloudinit"
	"github.com/juju/juju/environs"
)

// This interface  is added to allow to customize openstack provider behaviour.
// This is used in other providers, that embeds openstack provider.
type OpenstackProviderConfigurator interface {

	// Specify, whether openstack provider should use securiity group.
	// This is used in providers, that are based on openstack, but blocks security groups functionality.
	UseSecurityGroups() bool

	// Set of initial networks, that should be added by default to all new instances.
	InitialNetworks() []nova.ServerNetworks

	// This method allows to adjust defult RunServerOptions, before new server is actually created.
	ModifyRunServerOptions(options *nova.RunServerOpts)

	// This method provides default cloud config.
	// This config can be defferent for different providers.
	GetCloudConfig(args environs.StartInstanceParams) (cloudinit.CloudConfig, error)
}

type defaultProviderConfigurator struct{}

// UseSecurityGroups implements OpenstackProviderConfigurator interface.
func (c *defaultProviderConfigurator) UseSecurityGroups() bool {
	return true
}

// InitialNetworks implements OpenstackProviderConfigurator interface.
func (c *defaultProviderConfigurator) InitialNetworks() []nova.ServerNetworks {
	return []nova.ServerNetworks{}
}

// ModifyRunServerOptions implements OpenstackProviderConfigurator interface.
func (c *defaultProviderConfigurator) ModifyRunServerOptions(options *nova.RunServerOpts) {
}

// GetCloudConfig implements OpenstackProviderConfigurator interface.
func (c *defaultProviderConfigurator) GetCloudConfig(args environs.StartInstanceParams) (cloudinit.CloudConfig, error) {
	return nil, nil
}
