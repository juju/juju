<<<<<<< HEAD
<<<<<<< HEAD
// Copyright 2015 Canonical Ltd.
=======
// Copyright 2012, 2013 Canonical Ltd.
>>>>>>> modifications to opestack provider applied
=======
// Copyright 2015 Canonical Ltd.
>>>>>>> review comments implemented
// Licensed under the AGPLv3, see LICENCE file for details.

// Stub provider for OpenStack, using goose will be implemented here

package openstack

import (
	"gopkg.in/goose.v1/nova"
<<<<<<< HEAD
<<<<<<< HEAD
=======
>>>>>>> working version of rackspace provider

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
<<<<<<< HEAD
<<<<<<< HEAD
=======
>>>>>>> review comments implemented

	// This method provides default cloud config.
	// This config can be defferent for different providers.
	GetCloudConfig(args environs.StartInstanceParams) (cloudinit.CloudConfig, error)
=======
)

type OpenstackProviderConfigurator interface {
	UseSecurityGroups() bool
	InitialNetworks() []nova.ServerNetworks
	ModifyRunServerOptions(options *nova.RunServerOpts)
>>>>>>> modifications to opestack provider applied
=======
	GetCloudConfig(args environs.StartInstanceParams) (cloudinit.CloudConfig, error)
>>>>>>> working version of rackspace provider
}

type defaultProviderConfigurator struct{}

<<<<<<< HEAD
<<<<<<< HEAD
// UseSecurityGroups implements OpenstackProviderConfigurator interface.
=======
>>>>>>> modifications to opestack provider applied
=======
// UseSecurityGroups implements OpenstackProviderConfigurator interface.
>>>>>>> review comments implemented
func (c *defaultProviderConfigurator) UseSecurityGroups() bool {
	return true
}

<<<<<<< HEAD
<<<<<<< HEAD
// InitialNetworks implements OpenstackProviderConfigurator interface.
=======
>>>>>>> modifications to opestack provider applied
=======
// InitialNetworks implements OpenstackProviderConfigurator interface.
>>>>>>> review comments implemented
func (c *defaultProviderConfigurator) InitialNetworks() []nova.ServerNetworks {
	return []nova.ServerNetworks{}
}

<<<<<<< HEAD
<<<<<<< HEAD
// ModifyRunServerOptions implements OpenstackProviderConfigurator interface.
func (c *defaultProviderConfigurator) ModifyRunServerOptions(options *nova.RunServerOpts) {
}

// GetCloudConfig implements OpenstackProviderConfigurator interface.
func (c *defaultProviderConfigurator) GetCloudConfig(args environs.StartInstanceParams) (cloudinit.CloudConfig, error) {
	return nil, nil
}
=======
=======
// ModifyRunServerOptions implements OpenstackProviderConfigurator interface.
>>>>>>> review comments implemented
func (c *defaultProviderConfigurator) ModifyRunServerOptions(options *nova.RunServerOpts) {
}
<<<<<<< HEAD
>>>>>>> modifications to opestack provider applied
=======

// GetCloudConfig implements OpenstackProviderConfigurator interface.
func (c *defaultProviderConfigurator) GetCloudConfig(args environs.StartInstanceParams) (cloudinit.CloudConfig, error) {
	return nil, nil
}
>>>>>>> working version of rackspace provider
