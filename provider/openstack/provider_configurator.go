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
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/juju/errors"
	"github.com/juju/utils"
	gooseerrors "gopkg.in/goose.v1/errors"
	"gopkg.in/goose.v1/nova"
<<<<<<< HEAD
<<<<<<< HEAD
=======
>>>>>>> working version of rackspace provider

	"github.com/juju/juju/cloudconfig/cloudinit"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/network"
)

// This interface  is added to allow to customize openstack provider behaviour.
// This is used in other providers, that embeds openstack provider.
type OpenstackProviderConfigurator interface {
	// OpenPorts opens the given port ranges for the whole environment.
	OpenPorts(ports []network.PortRange) error

	// ClosePorts closes the given port ranges for the whole environment.
	ClosePorts(ports []network.PortRange) error

	// Ports returns the port ranges opened for the whole environment.
	Ports() ([]network.PortRange, error)

	//Implementations shoud delete all global security groups.
	DeleteGlobalGroups() error

	// Implementations should return list of security groups, that belong to given instances.
	GetSecurityGroups(ids ...instance.Id) ([]string, error)

	// Implementations should set up initial security groups, if any.
	SetUpGroups(machineId string, apiPort int) ([]nova.SecurityGroup, error)

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

<<<<<<< HEAD
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
=======
type defaultProviderConfigurator struct {
	environ *Environ
>>>>>>> security group related methods moved to provider configurator
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
<<<<<<< HEAD
>>>>>>> working version of rackspace provider
=======

// setUpGroups creates the security groups for the new machine, and
// returns them.
//
// Instances are tagged with a group so they can be distinguished from
// other instances that might be running on the same OpenStack account.
// In addition, a specific machine security group is created for each
// machine, so that its firewall rules can be configured per machine.
//
// Note: ideally we'd have a better way to determine group membership so that 2
// people that happen to share an openstack account and name their environment
// "openstack" don't end up destroying each other's machines.
func (c *defaultProviderConfigurator) SetUpGroups(machineId string, apiPort int) ([]nova.SecurityGroup, error) {
	jujuGroup, err := c.setUpGlobalGroup(c.environ.jujuGroupName(), apiPort)
	if err != nil {
		return nil, err
	}
	var machineGroup nova.SecurityGroup
	switch c.environ.Config().FirewallMode() {
	case config.FwInstance:
		machineGroup, err = c.ensureGroup(c.environ.machineGroupName(machineId), nil)
	case config.FwGlobal:
		machineGroup, err = c.ensureGroup(c.environ.globalGroupName(), nil)
	}
	if err != nil {
		return nil, err
	}
	groups := []nova.SecurityGroup{jujuGroup, machineGroup}
	if c.environ.ecfg().useDefaultSecurityGroup() {
		defaultGroup, err := c.environ.nova().SecurityGroupByName("default")
		if err != nil {
			return nil, fmt.Errorf("loading default security group: %v", err)
		}
		groups = append(groups, *defaultGroup)
	}
	return groups, nil
}
func (c *defaultProviderConfigurator) setUpGlobalGroup(groupName string, apiPort int) (nova.SecurityGroup, error) {
	return c.ensureGroup(groupName,
		[]nova.RuleInfo{
			{
				IPProtocol: "tcp",
				FromPort:   22,
				ToPort:     22,
				Cidr:       "0.0.0.0/0",
			},
			{
				IPProtocol: "tcp",
				FromPort:   apiPort,
				ToPort:     apiPort,
				Cidr:       "0.0.0.0/0",
			},
			{
				IPProtocol: "tcp",
				FromPort:   1,
				ToPort:     65535,
			},
			{
				IPProtocol: "udp",
				FromPort:   1,
				ToPort:     65535,
			},
			{
				IPProtocol: "icmp",
				FromPort:   -1,
				ToPort:     -1,
			},
		})
}

// zeroGroup holds the zero security group.
var zeroGroup nova.SecurityGroup

// ensureGroup returns the security group with name and perms.
// If a group with name does not exist, one will be created.
// If it exists, its permissions are set to perms.
func (c *defaultProviderConfigurator) ensureGroup(name string, rules []nova.RuleInfo) (nova.SecurityGroup, error) {
	novaClient := c.environ.nova()
	// First attempt to look up an existing group by name.
	group, err := novaClient.SecurityGroupByName(name)
	if err == nil {
		// Group exists, so assume it is correctly set up and return it.
		// TODO(jam): 2013-09-18 http://pad.lv/121795
		// We really should verify the group is set up correctly,
		// because deleting and re-creating environments can get us bad
		// groups (especially if they were set up under Python)
		return *group, nil
	}
	// Doesn't exist, so try and create it.
	group, err = novaClient.CreateSecurityGroup(name, "juju group")
	if err != nil {
		if !gooseerrors.IsDuplicateValue(err) {
			return zeroGroup, err
		} else {
			// We just tried to create a duplicate group, so load the existing group.
			group, err = novaClient.SecurityGroupByName(name)
			if err != nil {
				return zeroGroup, err
			}
			return *group, nil
		}
	}
	// The new group is created so now add the rules.
	group.Rules = make([]nova.SecurityGroupRule, len(rules))
	for i, rule := range rules {
		rule.ParentGroupId = group.Id
		if rule.Cidr == "" {
			// http://pad.lv/1226996 Rules that don't have a CIDR
			// are meant to apply only to this group. If you don't
			// supply CIDR or GroupId then openstack assumes you
			// mean CIDR=0.0.0.0/0
			rule.GroupId = &group.Id
		}
		groupRule, err := novaClient.CreateSecurityGroupRule(rule)
		if err != nil && !gooseerrors.IsDuplicateValue(err) {
			return zeroGroup, err
		}
		group.Rules[i] = *groupRule
	}
	return *group, nil
}

// GetSecurityGroups implements OpenstackProviderConfigurator interface.
func (c *defaultProviderConfigurator) GetSecurityGroups(ids ...instance.Id) ([]string, error) {
	var securityGroupNames []string
	if c.environ.Config().FirewallMode() == config.FwInstance {
		instances, err := c.environ.Instances(ids)
		if err != nil {
			return nil, err
		}
		securityGroupNames = make([]string, 0, len(ids))
		for _, inst := range instances {
			if inst == nil {
				continue
			}
			openstackName := inst.(*openstackInstance).getServerDetail().Name
			lastDashPos := strings.LastIndex(openstackName, "-")
			if lastDashPos == -1 {
				return nil, fmt.Errorf("cannot identify machine ID in openstack server name %q", openstackName)
			}
			securityGroupName := c.environ.machineGroupName(openstackName[lastDashPos+1:])
			securityGroupNames = append(securityGroupNames, securityGroupName)
		}
	}
	return securityGroupNames, nil
}

// DeleteglobalGroups implements OpenstackProviderConfigurator interface.
func (c *defaultProviderConfigurator) DeleteGlobalGroups() error {
	novaClient := c.environ.nova()
	securityGroups, err := novaClient.ListSecurityGroups()
	if err != nil {
		return errors.Annotate(err, "cannot list security groups")
	}
	re, err := regexp.Compile(fmt.Sprintf("^%s(-\\d+)?$", c.environ.jujuGroupName()))
	if err != nil {
		return errors.Trace(err)
	}
	globalGroupName := c.environ.globalGroupName()
	for _, group := range securityGroups {
		if re.MatchString(group.Name) || group.Name == globalGroupName {
			deleteSecurityGroup(novaClient, group.Name, group.Id)
		}
	}
	return nil
}

// deleteSecurityGroup attempts to delete the security group. Should it fail,
// the deletion is retried due to timing issues in openstack. A security group
// cannot be deleted while it is in use. Theoretically we terminate all the
// instances before we attempt to delete the associated security groups, but
// in practice nova hasn't always finished with the instance before it
// returns, so there is a race condition where we think the instance is
// terminated and hence attempt to delete the security groups but nova still
// has it around internally. To attempt to catch this timing issue, deletion
// of the groups is tried multiple times.
func deleteSecurityGroup(novaclient *nova.Client, name, id string) {
	attempts := utils.AttemptStrategy{
		Total: 30 * time.Second,
		Delay: time.Second,
	}
	logger.Debugf("deleting security group %q", name)
	i := 0
	for attempt := attempts.Start(); attempt.Next(); {
		err := novaclient.DeleteSecurityGroup(id)
		if err == nil {
			return
		}
		i++
		if i%4 == 0 {
			message := fmt.Sprintf("waiting to delete security group %q", name)
			if i != 4 {
				message = "still " + message
			}
			logger.Debugf(message)
		}
	}
	logger.Warningf("cannot delete security group %q. Used by another environment?", name)
}

// OpenPorts implements OpenstackProviderConfigurator interface.
func (c *defaultProviderConfigurator) OpenPorts(ports []network.PortRange) error {
	if c.environ.Config().FirewallMode() != config.FwGlobal {
		return fmt.Errorf("invalid firewall mode %q for opening ports on environment",
			c.environ.Config().FirewallMode())
	}
	if err := c.environ.openPortsInGroup(c.environ.globalGroupName(), ports); err != nil {
		return err
	}
	logger.Infof("opened ports in global group: %v", ports)
	return nil
}

// ClosePorts implements OpenstackProviderConfigurator interface.
func (c *defaultProviderConfigurator) ClosePorts(ports []network.PortRange) error {
	if c.environ.Config().FirewallMode() != config.FwGlobal {
		return fmt.Errorf("invalid firewall mode %q for closing ports on environment",
			c.environ.Config().FirewallMode())
	}
	if err := c.environ.closePortsInGroup(c.environ.globalGroupName(), ports); err != nil {
		return err
	}
	logger.Infof("closed ports in global group: %v", ports)
	return nil
}

// Ports implements OpenstackProviderConfigurator interface.
func (c *defaultProviderConfigurator) Ports() ([]network.PortRange, error) {
	if c.environ.Config().FirewallMode() != config.FwGlobal {
		return nil, fmt.Errorf("invalid firewall mode %q for retrieving ports from environment",
			c.environ.Config().FirewallMode())
	}
	return c.environ.portsInGroup(c.environ.globalGroupName())
}

func (c *defaultProviderConfigurator) globalGroupName() string {
	return fmt.Sprintf("%s-global", c.environ.jujuGroupName())
}
>>>>>>> security group related methods moved to provider configurator
