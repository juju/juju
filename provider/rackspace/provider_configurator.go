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

<<<<<<< HEAD
	"github.com/juju/juju/cloudconfig/cloudinit"
	"github.com/juju/juju/environs"
<<<<<<< HEAD
<<<<<<< HEAD
<<<<<<< HEAD
=======
	"gopkg.in/goose.v1/nova"
>>>>>>> modifications to opestack provider applied
=======
>>>>>>> working version of rackspace provider
=======
	"github.com/juju/juju/instance"
	"github.com/juju/juju/network"
<<<<<<< HEAD
>>>>>>> security group related methods moved to provider configurator
=======
	"github.com/juju/juju/provider/openstack"
>>>>>>> Fix trivial issues in docstrings and layout
)

type rackspaceProviderConfigurator struct{}

<<<<<<< HEAD
<<<<<<< HEAD
<<<<<<< HEAD
<<<<<<< HEAD
=======
>>>>>>> review comments implemented
// UseSecurityGroups implements OpenstackProviderConfigurator interface.
func (c *rackspaceProviderConfigurator) UseSecurityGroups() bool {
	// for now rackspace don't fully suport security groups functionality http://www.rackspace.com/knowledge_center/frequently-asked-question/security-groups-faq#whatisbeingLaunched
	return false
}

=======
>>>>>>> security group related methods moved to provider configurator
// InitialNetworks implements OpenstackProviderConfigurator interface.
func (c *rackspaceProviderConfigurator) InitialNetworks() []nova.ServerNetworks {
	// this are default racksapace networks http://docs.rackspace.com/servers/api/v2/cs-devguide/content/provision_server_with_networks.html
<<<<<<< HEAD
=======
func (c *rackspaceProviderConfigurator) UseSecurityGroups() bool {
	return false
}

func (c *rackspaceProviderConfigurator) InitialNetworks() []nova.ServerNetworks {
>>>>>>> modifications to opestack provider applied
=======
>>>>>>> review comments implemented
=======
var _ openstack.OpenstackProviderConfigurator = (*rackspaceProviderConfigurator)(nil)

// InitialNetworks implements OpenstackProviderConfigurator interface.
func (c *rackspaceProviderConfigurator) InitialNetworks() []nova.ServerNetworks {
	// These are the default rackspace networks, see:
	// http://docs.rackspace.com/servers/api/v2/cs-devguide/content/provision_server_with_networks.html
>>>>>>> Fix trivial issues in docstrings and layout
	return []nova.ServerNetworks{
		{NetworkId: "00000000-0000-0000-0000-000000000000"},
		{NetworkId: "11111111-1111-1111-1111-111111111111"},
	}
}

<<<<<<< HEAD
<<<<<<< HEAD
// ModifyRunServerOptions implements OpenstackProviderConfigurator interface.
func (c *rackspaceProviderConfigurator) ModifyRunServerOptions(options *nova.RunServerOpts) {
=======
	"github.com/juju/juju/provider/openstack"
=======
	"github.com/juju/juju/instance"
	"github.com/juju/juju/network"
	"github.com/juju/juju/provider/common"
>>>>>>> renamed files for avoiding megre conflicts, when syncronizing with upstream
)

type rackspaceFirewaller struct{}

// InitialNetworks implements OpenstackFirewaller interface.
func (c *rackspaceFirewaller) InitialNetworks() []nova.ServerNetworks {
	// this are default racksapace networks http://docs.rackspace.com/servers/api/v2/cs-devguide/content/provision_server_with_networks.html
	return []nova.ServerNetworks{
		{NetworkId: "00000000-0000-0000-0000-000000000000"},
		{NetworkId: "11111111-1111-1111-1111-111111111111"},
	}
}

// OpenPorts opens the given port ranges for the whole environment.
// Must only be used if the environment was setup with the
// FwGlobal firewall mode.
func (c *rackspaceFirewaller) OpenPorts(ports []network.PortRange) error {
	return errors.Trace(errors.NotSupportedf("ClosePorts"))
}

// ClosePorts closes the given port ranges for the whole environment.
// Must only be used if the environment was setup with the
// FwGlobal firewall mode.
func (c *rackspaceFirewaller) ClosePorts(ports []network.PortRange) error {
	return errors.Trace(errors.NotSupportedf("ClosePorts"))
}

// Ports returns the port ranges opened for the whole environment.
// Must only be used if the environment was setup with the
// FwGlobal firewall mode.
func (c *rackspaceFirewaller) Ports() ([]network.PortRange, error) {
	return nil, errors.Trace(errors.NotSupportedf("Ports"))
}

// DeleteglobalGroups implements OpenstackFirewaller interface.
func (c *rackspaceFirewaller) DeleteGlobalGroups() error {
	return nil
}

// GetSecurityGroups implements OpenstackFirewaller interface.
func (c *rackspaceFirewaller) GetSecurityGroups(ids ...instance.Id) ([]string, error) {
	return nil, nil
}

// SetUpGroups implements OpenstackFirewaller interface.
func (c *rackspaceFirewaller) SetUpGroups(machineId string, apiPort int) ([]nova.SecurityGroup, error) {
	return nil, nil
}

<<<<<<< HEAD
// ModifyRunServerOptions implements ProviderConfigurator interface.
func (c *rackspaceConfigurator) ModifyRunServerOptions(options *nova.RunServerOpts) {
>>>>>>> Firewaller interface added, Waith ssh method reused
	// more on how ConfigDrive option is used on rackspace http://docs.rackspace.com/servers/api/v2/cs-devguide/content/config_drive_ext.html
	options.ConfigDrive = true
=======
// OpenInstancePorts implements Firewaller interface.
func (c *rackspaceFirewaller) OpenInstancePorts(inst instance.Instance, machineId string, ports []network.PortRange) error {
	return c.changePorts(inst, true, ports)
>>>>>>> renamed files for avoiding megre conflicts, when syncronizing with upstream
}

// CloseInstancePorts implements Firewaller interface.
func (c *rackspaceFirewaller) CloseInstancePorts(inst instance.Instance, machineId string, ports []network.PortRange) error {
	return c.changePorts(inst, false, ports)
}

// InstancePorts implements Firewaller interface.
func (c *rackspaceFirewaller) InstancePorts(inst instance.Instance, machineId string) ([]network.PortRange, error) {
	_, configurator, err := c.getInstanceConfigurator(inst)
	if err != nil {
		return nil, errors.Trace(err)
	}
<<<<<<< HEAD
	// we need this package for sshInstanceConfigurator, to save iptables state between restarts
	cloudcfg.AddPackage("iptables-persistent")
	return cloudcfg, nil
}
=======
=======
// ModifyRunServerOptions implements OpenstackProviderConfigurator interface.
>>>>>>> review comments implemented
func (c *rackspaceProviderConfigurator) ModifyRunServerOptions(options *nova.RunServerOpts) {
	// More on how ConfigDrive option is used on rackspace:
	// http://docs.rackspace.com/servers/api/v2/cs-devguide/content/config_drive_ext.html
	options.ConfigDrive = true
}
<<<<<<< HEAD
>>>>>>> modifications to opestack provider applied
=======

<<<<<<< HEAD
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
<<<<<<< HEAD
>>>>>>> working version of rackspace provider
=======

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
=======
// GetConfigDefaults implements ProviderConfigurator interface.
func (c *rackspaceConfigurator) GetConfigDefaults() schema.Defaults {
	return schema.Defaults{
		"username":             "",
		"password":             "",
		"tenant-name":          "",
		"auth-url":             "https://identity.api.rackspacecloud.com/v2.0",
		"auth-mode":            string(openstack.AuthUserPass),
		"access-key":           "",
		"secret-key":           "",
		"region":               "",
		"control-bucket":       "",
		"use-floating-ip":      false,
		"use-default-secgroup": false,
		"network":              "",
	}
>>>>>>> Firewaller interface added, Waith ssh method reused
=======
	return configurator.FindOpenPorts()
}

func (c *rackspaceFirewaller) changePorts(inst instance.Instance, insert bool, ports []network.PortRange) error {
	addresses, sshClient, err := c.getInstanceConfigurator(inst)
	if err != nil {
		return errors.Trace(err)
	}

	for _, addr := range addresses {
		if addr.Scope == network.ScopePublic {
			err = sshClient.ChangePorts(addr.Value, insert, ports)
			if err != nil {
				return errors.Trace(err)
			}
		}
	}
	return nil
}

func (c *rackspaceFirewaller) getInstanceConfigurator(inst instance.Instance) ([]network.Address, common.InstanceConfigurator, error) {
	addresses, err := inst.Addresses()
	if err != nil {
		return nil, nil, errors.Trace(err)
	}
	if len(addresses) == 0 {
		return addresses, nil, errors.New("No addresses found")
	}

	client := common.NewSshInstanceConfigurator(addresses[0].Value)
	return addresses, client, err
>>>>>>> renamed files for avoiding megre conflicts, when syncronizing with upstream
}
>>>>>>> security group related methods moved to provider configurator
