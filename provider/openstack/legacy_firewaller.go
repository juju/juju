// Copyright 2015-2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package openstack

import (
	"fmt"
	"regexp"

	"github.com/juju/clock"
	"github.com/juju/errors"
	gooseerrors "gopkg.in/goose.v2/errors"
	"gopkg.in/goose.v2/neutron"
	"gopkg.in/goose.v2/nova"

	corenetwork "github.com/juju/juju/core/network"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/environs/context"
	"github.com/juju/juju/environs/instances"
	"github.com/juju/juju/network"
	"github.com/juju/juju/provider/common"
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
func (c *legacyNovaFirewaller) SetUpGroups(ctx context.ProviderCallContext, controllerUUID, machineID string, apiPort int) ([]string, error) {
	jujuGroup, err := c.setUpGlobalGroup(ctx, c.jujuGroupName(controllerUUID), apiPort)
	if err != nil {
		return nil, errors.Trace(err)
	}
	var machineGroup nova.SecurityGroup
	switch c.environ.Config().FirewallMode() {
	case config.FwInstance:
		machineGroup, err = c.ensureGroup(ctx, c.machineGroupName(controllerUUID, machineID), nil)
	case config.FwGlobal:
		machineGroup, err = c.ensureGroup(ctx, c.globalGroupName(controllerUUID), nil)
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

func (c *legacyNovaFirewaller) setUpGlobalGroup(ctx context.ProviderCallContext, groupName string, apiPort int) (nova.SecurityGroup, error) {
	return c.ensureGroup(ctx, groupName,
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
func (c *legacyNovaFirewaller) ensureGroup(ctx context.ProviderCallContext, name string, rules []nova.RuleInfo) (nova.SecurityGroup, error) {
	novaClient := c.environ.nova()
	// First attempt to look up an existing group by name.
	group, err := novaClient.SecurityGroupByName(name)
	if err == nil {
		common.HandleCredentialError(IsAuthorisationFailure, err, ctx)
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
				common.HandleCredentialError(IsAuthorisationFailure, err, ctx)
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
			common.HandleCredentialError(IsAuthorisationFailure, err, ctx)
			return legacyZeroGroup, err
		}
		group.Rules[i] = *groupRule
	}
	return *group, nil
}

func (c *legacyNovaFirewaller) deleteSecurityGroups(ctx context.ProviderCallContext, match func(name string) bool) error {
	novaclient := c.environ.nova()
	securityGroups, err := novaclient.ListSecurityGroups()
	if err != nil {
		common.HandleCredentialError(IsAuthorisationFailure, err, ctx)
		return errors.Annotate(err, "cannot list security groups")
	}

	for _, group := range securityGroups {
		if match(group.Name) {
			deleteSecurityGroup(ctx,
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
func (c *legacyNovaFirewaller) DeleteAllControllerGroups(ctx context.ProviderCallContext, controllerUUID string) error {
	return deleteSecurityGroupsMatchingName(ctx, c.deleteSecurityGroups, c.jujuControllerGroupPrefix(controllerUUID))
}

// DeleteAllModelGroups implements Firewaller interface.
func (c *legacyNovaFirewaller) DeleteAllModelGroups(ctx context.ProviderCallContext) error {
	return deleteSecurityGroupsMatchingName(ctx, c.deleteSecurityGroups, c.jujuGroupRegexp())
}

// DeleteGroups implements Firewaller interface.
func (c *legacyNovaFirewaller) DeleteGroups(ctx context.ProviderCallContext, names ...string) error {
	return deleteSecurityGroupsOneOfNames(ctx, c.deleteSecurityGroups, names...)
}

// UpdateGroupController implements Firewaller interface.
func (c *legacyNovaFirewaller) UpdateGroupController(ctx context.ProviderCallContext, controllerUUID string) error {
	novaClient := c.environ.nova()
	groups, err := novaClient.ListSecurityGroups()
	if err != nil {
		common.HandleCredentialError(IsAuthorisationFailure, err, ctx)
		return errors.Trace(err)
	}
	re, err := regexp.Compile(c.jujuGroupRegexp())
	if err != nil {
		return errors.Trace(err)
	}

	var failed []string
	for _, group := range groups {
		if !re.MatchString(group.Name) {
			continue
		}
		err := c.updateGroupControllerUUID(ctx, &group, controllerUUID)
		if err != nil {
			logger.Errorf("error updating controller for security group %s: %v", group.Id, err)
			failed = append(failed, group.Id)
			if denied := common.MaybeHandleCredentialError(IsAuthorisationFailure, err, ctx); denied {
				// We will keep failing 100% once the credential is deemed invalid - no point in persisting.
				break
			}
		}
	}
	if len(failed) != 0 {
		return errors.Errorf("errors updating controller for security groups: %v", failed)
	}
	return nil
}

func (c *legacyNovaFirewaller) updateGroupControllerUUID(ctx context.ProviderCallContext, group *nova.SecurityGroup, controllerUUID string) error {
	newName, err := replaceControllerUUID(group.Name, controllerUUID)
	if err != nil {
		return errors.Trace(err)
	}
	client := c.environ.nova()
	_, err = client.UpdateSecurityGroup(group.Id, newName, group.Description)
	common.HandleCredentialError(IsAuthorisationFailure, err, ctx)
	return errors.Trace(err)
}

// OpenPorts implements Firewaller interface.
func (c *legacyNovaFirewaller) OpenPorts(ctx context.ProviderCallContext, rules []network.IngressRule) error {
	return c.openPorts(ctx, c.openPortsInGroup, rules)
}

// ClosePorts implements Firewaller interface.
func (c *legacyNovaFirewaller) ClosePorts(ctx context.ProviderCallContext, rules []network.IngressRule) error {
	return c.closePorts(ctx, c.closePortsInGroup, rules)
}

// IngressRules implements Firewaller interface.
func (c *legacyNovaFirewaller) IngressRules(ctx context.ProviderCallContext) ([]network.IngressRule, error) {
	return c.ingressRules(ctx, c.ingressRulesInGroup)
}

// OpenInstancePorts implements Firewaller interface.
func (c *legacyNovaFirewaller) OpenInstancePorts(ctx context.ProviderCallContext, inst instances.Instance, machineID string, rules []network.IngressRule) error {
	return c.openInstancePorts(ctx, c.openPortsInGroup, machineID, rules)
}

// CloseInstancePorts implements Firewaller interface.
func (c *legacyNovaFirewaller) CloseInstancePorts(ctx context.ProviderCallContext, inst instances.Instance, machineID string, rules []network.IngressRule) error {
	return c.closeInstancePorts(ctx, c.closePortsInGroup, machineID, rules)
}

// InstanceIngressRules implements Firewaller interface.
func (c *legacyNovaFirewaller) InstanceIngressRules(ctx context.ProviderCallContext, inst instances.Instance, machineID string) ([]network.IngressRule, error) {
	return c.instanceIngressRules(ctx, c.ingressRulesInGroup, machineID)
}

func (c *legacyNovaFirewaller) matchingGroup(ctx context.ProviderCallContext, nameRegExp string) (nova.SecurityGroup, error) {
	re, err := regexp.Compile(nameRegExp)
	if err != nil {
		return nova.SecurityGroup{}, err
	}
	novaclient := c.environ.nova()
	allGroups, err := novaclient.ListSecurityGroups()
	if err != nil {
		common.HandleCredentialError(IsAuthorisationFailure, err, ctx)
		return nova.SecurityGroup{}, err
	}
	var matchingGroups []nova.SecurityGroup
	for _, group := range allGroups {
		if re.MatchString(group.Name) {
			matchingGroups = append(matchingGroups, group)
		}
	}
	numMatching := len(matchingGroups)
	if numMatching == 0 {
		return nova.SecurityGroup{}, errors.NotFoundf("security groups matching %q", nameRegExp)
	} else if numMatching > 1 {
		return nova.SecurityGroup{}, errors.New(fmt.Sprintf("%d security groups found matching %q, expected 1", numMatching, nameRegExp))
	}
	return matchingGroups[0], nil
}

func (c *legacyNovaFirewaller) openPortsInGroup(ctx context.ProviderCallContext, nameRegExp string, rules []network.IngressRule) error {
	group, err := c.matchingGroup(ctx, nameRegExp)
	if err != nil {
		return errors.Trace(err)
	}
	novaclient := c.environ.nova()
	ruleInfo := rulesToRuleInfo(group.Id, rules)
	for _, rule := range ruleInfo {
		_, err := novaclient.CreateSecurityGroupRule(legacyRuleInfo(rule))
		if err != nil {
			common.HandleCredentialError(IsAuthorisationFailure, err, ctx)
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
func legacyRuleMatchesPortRange(rule nova.SecurityGroupRule, portRange network.IngressRule) bool {
	if rule.IPProtocol == nil || rule.FromPort == nil || rule.ToPort == nil {
		return false
	}
	return *rule.IPProtocol == portRange.Protocol &&
		*rule.FromPort == portRange.FromPort &&
		*rule.ToPort == portRange.ToPort
}

func (c *legacyNovaFirewaller) closePortsInGroup(ctx context.ProviderCallContext, nameRegExp string, rules []network.IngressRule) error {
	if len(rules) == 0 {
		return nil
	}
	group, err := c.matchingGroup(ctx, nameRegExp)
	if err != nil {
		return errors.Trace(err)
	}
	novaclient := c.environ.nova()
	for _, portRange := range rules {
		for _, p := range group.Rules {
			if !legacyRuleMatchesPortRange(p, portRange) {
				continue
			}
			err := novaclient.DeleteSecurityGroupRule(p.Id)
			if err != nil {
				common.HandleCredentialError(IsAuthorisationFailure, err, ctx)
				return errors.Trace(err)
			}
			break
		}
	}
	return nil
}

func (c *legacyNovaFirewaller) ingressRulesInGroup(ctx context.ProviderCallContext, nameRegexp string) (rules []network.IngressRule, err error) {
	group, err := c.matchingGroup(ctx, nameRegexp)
	if err != nil {
		return nil, errors.Trace(err)
	}
	// Keep track of all the RemoteIPPrefixes for each port range.
	portSourceCIDRs := make(map[corenetwork.PortRange]*[]string)
	for _, p := range group.Rules {
		portRange := corenetwork.PortRange{*p.FromPort, *p.ToPort, *p.IPProtocol}
		// Record the RemoteIPPrefix for the port range.
		remotePrefix := p.IPRange["cidr"]
		if remotePrefix == "" {
			remotePrefix = "0.0.0.0/0"
		}
		sourceCIDRs, ok := portSourceCIDRs[portRange]
		if !ok {
			sourceCIDRs = &[]string{}
			portSourceCIDRs[portRange] = sourceCIDRs
		}
		*sourceCIDRs = append(*sourceCIDRs, remotePrefix)
	}
	// Combine all the port ranges and remote prefixes.
	for portRange, sourceCIDRs := range portSourceCIDRs {
		rule, err := network.NewIngressRule(
			portRange.Protocol,
			portRange.FromPort,
			portRange.ToPort,
			*sourceCIDRs...)
		if err != nil {
			common.HandleCredentialError(IsAuthorisationFailure, err, ctx)
			return nil, errors.Trace(err)
		}
		rules = append(rules, rule)
	}
	network.SortIngressRules(rules)
	return rules, nil
}
