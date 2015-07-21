// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package rackspace

import (
<<<<<<< HEAD
<<<<<<< HEAD
=======
>>>>>>> working version of rackspace provider
	"github.com/juju/errors"
	"gopkg.in/goose.v1/nova"

	"github.com/juju/juju/cloudconfig/cloudinit"
	"github.com/juju/juju/environs"
<<<<<<< HEAD
=======
	"gopkg.in/goose.v1/nova"
>>>>>>> modifications to opestack provider applied
=======
>>>>>>> working version of rackspace provider
)

type rackspaceProviderConfigurator struct{}

<<<<<<< HEAD
// UseSecurityGroups implements OpenstackProviderConfigurator interface.
func (c *rackspaceProviderConfigurator) UseSecurityGroups() bool {
	// for now rackspace don't fully suport security groups functionality http://www.rackspace.com/knowledge_center/frequently-asked-question/security-groups-faq#whatisbeingLaunched
	return false
}

// InitialNetworks implements OpenstackProviderConfigurator interface.
func (c *rackspaceProviderConfigurator) InitialNetworks() []nova.ServerNetworks {
	// this are default racksapace networks http://docs.rackspace.com/servers/api/v2/cs-devguide/content/provision_server_with_networks.html
=======
func (c *rackspaceProviderConfigurator) UseSecurityGroups() bool {
	return false
}

func (c *rackspaceProviderConfigurator) InitialNetworks() []nova.ServerNetworks {
>>>>>>> modifications to opestack provider applied
	return []nova.ServerNetworks{
		{NetworkId: "00000000-0000-0000-0000-000000000000"},
		{NetworkId: "11111111-1111-1111-1111-111111111111"},
	}
}

<<<<<<< HEAD
// ModifyRunServerOptions implements OpenstackProviderConfigurator interface.
func (c *rackspaceProviderConfigurator) ModifyRunServerOptions(options *nova.RunServerOpts) {
	// more on how ConfigDrive option is used on rackspace http://docs.rackspace.com/servers/api/v2/cs-devguide/content/config_drive_ext.html
	options.ConfigDrive = true
}

// GetCloudConfig implements OpenstackProviderConfigurator interface.
func (c *rackspaceProviderConfigurator) GetCloudConfig(args environs.StartInstanceParams) (cloudinit.CloudConfig, error) {
	cloudcfg, err := cloudinit.New(args.Tools.OneSeries())
	if err != nil {
		return nil, errors.Trace(err)
	}
	// we need this package for sshInstanceConfigurator, to save iptables state between restarts
	cloudcfg.AddPackage("iptables-persistent")
	return cloudcfg, nil
}
=======
func (c *rackspaceProviderConfigurator) ModifyRunServerOptions(options *nova.RunServerOpts) {
	options.ConfigDrive = true
}
<<<<<<< HEAD
>>>>>>> modifications to opestack provider applied
=======

func (c *rackspaceProviderConfigurator) GetCloudConfig(args environs.StartInstanceParams) (cloudinit.CloudConfig, error) {
	cloudcfg, err := cloudinit.New(args.Tools.OneSeries())
	if err != nil {
		return nil, errors.Trace(err)
	}
	cloudcfg.AddPackage("iptables-persistent")
	return cloudcfg, nil
}
>>>>>>> working version of rackspace provider
