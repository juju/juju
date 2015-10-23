// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package rackspace

import (
	"github.com/juju/errors"
	"gopkg.in/goose.v1/nova"

	"github.com/juju/juju/cloudconfig/cloudinit"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/network"
	"github.com/juju/juju/provider/openstack"
)

type rackspaceProviderConfigurator struct{}

var _ openstack.OpenstackProviderConfigurator = (*rackspaceProviderConfigurator)(nil)

// InitialNetworks implements OpenstackProviderConfigurator interface.
func (c *rackspaceProviderConfigurator) InitialNetworks() []nova.ServerNetworks {
	// These are the default rackspace networks, see:
	// http://docs.rackspace.com/servers/api/v2/cs-devguide/content/provision_server_with_networks.html
	return []nova.ServerNetworks{
		{NetworkId: "00000000-0000-0000-0000-000000000000"},
		{NetworkId: "11111111-1111-1111-1111-111111111111"},
	}
}

// ModifyRunServerOptions implements OpenstackProviderConfigurator interface.
func (c *rackspaceProviderConfigurator) ModifyRunServerOptions(options *nova.RunServerOpts) {
	// More on how ConfigDrive option is used on rackspace:
	// http://docs.rackspace.com/servers/api/v2/cs-devguide/content/config_drive_ext.html
	options.ConfigDrive = true
}

// GetCloudConfig implements OpenstackProviderConfigurator interface.
func (c *rackspaceProviderConfigurator) GetCloudConfig(args environs.StartInstanceParams) (cloudinit.CloudConfig, error) {
	cloudcfg, err := cloudinit.New(args.Tools.OneSeries())
	if err != nil {
		return nil, errors.Trace(err)
	}
	// Additional package required for sshInstanceConfigurator, to save
	// iptables state between restarts.
	cloudcfg.AddPackage("iptables-persistent")
	return cloudcfg, nil
}

// OpenPorts is not supported.
func (c *rackspaceProviderConfigurator) OpenPorts(ports []network.PortRange) error {
	return errors.Trace(errors.NotSupportedf("ClosePorts"))
}

// ClosePorts is not supported.
func (c *rackspaceProviderConfigurator) ClosePorts(ports []network.PortRange) error {
	return errors.Trace(errors.NotSupportedf("ClosePorts"))
}

// Ports returns the port ranges opened for the whole environment.
// Must only be used if the environment was setup with the
// FwGlobal firewall mode.
func (c *rackspaceProviderConfigurator) Ports() ([]network.PortRange, error) {
	return nil, errors.Trace(errors.NotSupportedf("Ports"))
}

// DeleteGlobalGroups implements OpenstackProviderConfigurator interface.
func (c *rackspaceProviderConfigurator) DeleteGlobalGroups() error {
	return nil
}

// GetSecurityGroups implements OpenstackProviderConfigurator interface.
func (c *rackspaceProviderConfigurator) GetSecurityGroups(ids ...instance.Id) ([]string, error) {
	return nil, nil
}

// SetUpGroups implements OpenstackProviderConfigurator interface.
func (c *rackspaceProviderConfigurator) SetUpGroups(machineId string, apiPort int) ([]nova.SecurityGroup, error) {
	return nil, nil
}
