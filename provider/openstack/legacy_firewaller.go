// Copyright 2015-2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package openstack

import (
	"regexp"

	"github.com/juju/errors"
	"github.com/juju/utils/clock"
	gooseerrors "gopkg.in/goose.v1/errors"
	"gopkg.in/goose.v1/neutron"
	"gopkg.in/goose.v1/nova"

	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/network"
)

type legacyNovaFirewaller struct {
	firewallerBase
}

// SetUpGroups creates the security groups for the new machine, and
// returns them.
//
// Instances are tagged with a group so they can be distinguished from
// other instances that might be running on the same OpenStack account.
// In addition, a specific machine security group is created for each
// machine, so that its firewall rules can be configured per machine.
func (c *legacyNovaFirewaller) SetUpGroups(controllerUUID, machineId string, apiPort int) ([]string, error) {
	jujuGroup, err := c.setUpGlobalGroup(c.jujuGroupName(controllerUUID), apiPort)
	if err != nil {
		return nil, errors.Trace(err)
	}
	var machineGroup nova.SecurityGroup
	switch c.environ.Config().FirewallMode() {
	case config.FwInstance:
		machineGroup, err = c.ensureGroup(c.machineGroupName(controllerUUID, machineId), nil)
	case config.FwGlobal:
		machineGroup, err = c.ensureGroup(c.globalGroupName(controllerUUID), nil)
	}
	if err != nil {
		return nil, errors.Trace(err)
	}
	groupNames := []string{jujuGroup.Name, machineGroup.Name}
	if c.environ.ecfg().useDefaultSecurityGroup() {
		groupNames = append(groupNames, "default")
	}
	return groupNames, nil
}

func (c *legacyNovaFirewaller) setUpGlobalGroup(groupName string, apiPort int) (nova.SecurityGroup, error) {
	return c.ensureGroup(groupName,
		[]nova.RuleInfo{
			{
				IPProtocol: "tcp",
				ToPort:     22,
				FromPort:   22,
				Cidr:       "0.0.0.0/0",
			},
			{
				IPProtocol: "tcp",
				ToPort:     apiPort,
				FromPort:   apiPort,
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

// legacyZeroGroup holds the zero security group.
var legacyZeroGroup nova.SecurityGroup

// ensureGroup returns the security group with name and perms.
// If a group with name does not exist, one will be created.
// If it exists, its permissions are set to perms.
func (c *legacyNovaFirewaller) ensureGroup(name string, rules []nova.RuleInfo) (nova.SecurityGroup, error) {
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
			return legacyZeroGroup, err
		} else {
			// We just tried to create a duplicate group, so load the existing group.
			group, err = novaClient.SecurityGroupByName(name)
			if err != nil {
				return legacyZeroGroup, err
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
			return legacyZeroGroup, err
		}
		group.Rules[i] = *groupRule
	}
	return *group, nil
}

func (c *legacyNovaFirewaller) deleteSecurityGroups(match func(name string) bool) error {
	novaclient := c.environ.nova()
	securityGroups, err := novaclient.ListSecurityGroups()
	if err != nil {
		return errors.Annotate(err, "cannot list security groups")
	}
	for _, group := range securityGroups {
		if match(group.Name) {
			deleteSecurityGroup(
				novaclient.DeleteSecurityGroup,
				group.Name,
				group.Id,
				clock.WallClock,
			)
		}
	}
	return nil
}

// DeleteAllControllerGroups implements Firewaller interface.
func (c *legacyNovaFirewaller) DeleteAllControllerGroups(controllerUUID string) error {
	return deleteSecurityGroupsMatchingName(c.deleteSecurityGroups, c.jujuControllerGroupPrefix(controllerUUID))
}

// DeleteAllModelGroups implements Firewaller interface.
func (c *legacyNovaFirewaller) DeleteAllModelGroups() error {
	return deleteSecurityGroupsMatchingName(c.deleteSecurityGroups, c.jujuGroupRegexp())
}

// DeleteGroups implements Firewaller interface.
func (c *legacyNovaFirewaller) DeleteGroups(names ...string) error {
	return deleteSecurityGroupsOneOfNames(c.deleteSecurityGroups, names...)
}

// OpenPorts implements Firewaller interface.
func (c *legacyNovaFirewaller) OpenPorts(ports []network.PortRange) error {
	return c.openPorts(c.openPortsInGroup, ports)
}

// ClosePorts implements Firewaller interface.
func (c *legacyNovaFirewaller) ClosePorts(ports []network.PortRange) error {
	return c.closePorts(c.closePortsInGroup, ports)
}

// Ports implements Firewaller interface.
func (c *legacyNovaFirewaller) Ports() ([]network.PortRange, error) {
	return c.ports(c.portsInGroup)
}

// OpenInstancePorts implements Firewaller interface.
func (c *legacyNovaFirewaller) OpenInstancePorts(inst instance.Instance, machineId string, ports []network.PortRange) error {
	return c.openInstancePorts(c.openPortsInGroup, machineId, ports)
}

// CloseInstancePorts implements Firewaller interface.
func (c *legacyNovaFirewaller) CloseInstancePorts(inst instance.Instance, machineId string, ports []network.PortRange) error {
	return c.closeInstancePorts(c.closePortsInGroup, machineId, ports)
}

// InstancePorts implements Firewaller interface.
func (c *legacyNovaFirewaller) InstancePorts(inst instance.Instance, machineId string) ([]network.PortRange, error) {
	return c.instancePorts(c.portsInGroup, machineId)
}

func (c *legacyNovaFirewaller) matchingGroup(nameRegExp string) (nova.SecurityGroup, error) {
	re, err := regexp.Compile(nameRegExp)
	if err != nil {
		return nova.SecurityGroup{}, err
	}
	novaclient := c.environ.nova()
	allGroups, err := novaclient.ListSecurityGroups()
	if err != nil {
		return nova.SecurityGroup{}, err
	}
	var matchingGroups []nova.SecurityGroup
	for _, group := range allGroups {
		if re.MatchString(group.Name) {
			matchingGroups = append(matchingGroups, group)
		}
	}
	if len(matchingGroups) != 1 {
		return nova.SecurityGroup{}, errors.NotFoundf("security groups matching %q", nameRegExp)
	}
	return matchingGroups[0], nil
}

func (c *legacyNovaFirewaller) openPortsInGroup(nameRegExp string, portRanges []network.PortRange) error {
	group, err := c.matchingGroup(nameRegExp)
	if err != nil {
		return errors.Trace(err)
	}
	novaclient := c.environ.nova()
	rules := portsToRuleInfo(group.Id, portRanges)
	for _, rule := range rules {
		_, err := novaclient.CreateSecurityGroupRule(legacyRuleInfo(rule))
		if err != nil {
			// TODO: if err is not rule already exists, raise?
			logger.Debugf("error creating security group rule: %v", err.Error())
		}
	}
	return nil
}

func legacyRuleInfo(in neutron.RuleInfoV2) nova.RuleInfo {
	return nova.RuleInfo{
		ParentGroupId: in.ParentGroupId,
		FromPort:      in.PortRangeMin,
		ToPort:        in.PortRangeMax,
		IPProtocol:    in.IPProtocol,
		Cidr:          in.RemoteIPPrefix,
	}
}

// ruleMatchesPortRange checks if supplied nova security group rule matches the port range
func legacyRuleMatchesPortRange(rule nova.SecurityGroupRule, portRange network.PortRange) bool {
	if rule.IPProtocol == nil || rule.FromPort == nil || rule.ToPort == nil {
		return false
	}
	return *rule.IPProtocol == portRange.Protocol &&
		*rule.FromPort == portRange.FromPort &&
		*rule.ToPort == portRange.ToPort
}

func (c *legacyNovaFirewaller) closePortsInGroup(nameRegExp string, portRanges []network.PortRange) error {
	if len(portRanges) == 0 {
		return nil
	}
	group, err := c.matchingGroup(nameRegExp)
	if err != nil {
		return errors.Trace(err)
	}
	novaclient := c.environ.nova()
	for _, portRange := range portRanges {
		for _, p := range group.Rules {
			if !legacyRuleMatchesPortRange(p, portRange) {
				continue
			}
			err := novaclient.DeleteSecurityGroupRule(p.Id)
			if err != nil {
				return errors.Trace(err)
			}
			break
		}
	}
	return nil
}

func (c *legacyNovaFirewaller) portsInGroup(nameRegexp string) (portRanges []network.PortRange, err error) {
	group, err := c.matchingGroup(nameRegexp)
	if err != nil {
		return nil, errors.Trace(err)
	}
	for _, p := range group.Rules {
		portRanges = append(portRanges, network.PortRange{
			Protocol: *p.IPProtocol,
			FromPort: *p.FromPort,
			ToPort:   *p.ToPort,
		})
	}
	network.SortPortRanges(portRanges)
	return portRanges, nil
}
