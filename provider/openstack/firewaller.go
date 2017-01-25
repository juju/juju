// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package openstack

import (
	"fmt"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/juju/errors"
	"github.com/juju/retry"
	"github.com/juju/utils/clock"
	"gopkg.in/goose.v1/neutron"

	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/network"
)

//factory for obtaining firawaller object.
type FirewallerFactory interface {
	GetFirewaller(env environs.Environ) Firewaller
}

// Firewaller allows custom openstack provider behaviour.
// This is used in other providers that embed the openstack provider.
type Firewaller interface {
	// OpenPorts opens the given port ranges for the whole environment.
	OpenPorts(rules []network.IngressRule) error

	// ClosePorts closes the given port ranges for the whole environment.
	ClosePorts(rules []network.IngressRule) error

	// IngressRules returns the ingress rules applied to the whole environment.
	// It is expected that there be only one ingress rule result for a given
	// port range - the rule's SourceCIDRs will contain all applicable source
	// address rules for that port range.
	IngressRules() ([]network.IngressRule, error)

	// Implementations are expected to delete all security groups for the
	// environment.
	DeleteAllModelGroups() error

	// Implementations are expected to delete all security groups for the
	// controller, ie those for all hosted models.
	DeleteAllControllerGroups(controllerUUID string) error

	// DeleteGroups deletes the security groups with the specified names.
	DeleteGroups(names ...string) error

	// Implementations should return list of security groups, that belong to given instances.
	GetSecurityGroups(ids ...instance.Id) ([]string, error)

	// SetUpGroups sets up initial security groups, if any, and returns
	// their names.
	SetUpGroups(controllerUUID, machineId string, apiPort int) ([]string, error)

	// OpenInstancePorts opens the given port ranges for the specified  instance.
	OpenInstancePorts(inst instance.Instance, machineId string, rules []network.IngressRule) error

	// CloseInstancePorts closes the given port ranges for the specified  instance.
	CloseInstancePorts(inst instance.Instance, machineId string, rules []network.IngressRule) error

	// InstanceIngressRules returns the ingress rules applied to the specified  instance.
	InstanceIngressRules(inst instance.Instance, machineId string) ([]network.IngressRule, error)
}

type firewallerFactory struct {
}

// GetFirewaller implements FirewallerFactory
func (f *firewallerFactory) GetFirewaller(env environs.Environ) Firewaller {
	return &switchingFirewaller{env: env.(*Environ)}
}

type switchingFirewaller struct {
	env *Environ

	mu sync.Mutex
	fw Firewaller
}

func (f *switchingFirewaller) initFirewaller() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.fw != nil {
		return nil
	}

	client := f.env.client()
	if !client.IsAuthenticated() {
		if err := authenticateClient(client); err != nil {
			return errors.Trace(err)
		}
	}

	base := firewallerBase{environ: f.env}
	if f.env.supportsNeutron() {
		f.fw = &neutronFirewaller{base}
	} else {
		f.fw = &legacyNovaFirewaller{base}
	}
	return nil
}

func (f *switchingFirewaller) OpenPorts(rules []network.IngressRule) error {
	if err := f.initFirewaller(); err != nil {
		return errors.Trace(err)
	}
	return f.fw.OpenPorts(rules)
}

func (f *switchingFirewaller) ClosePorts(rules []network.IngressRule) error {
	if err := f.initFirewaller(); err != nil {
		return errors.Trace(err)
	}
	return f.fw.ClosePorts(rules)
}

func (f *switchingFirewaller) IngressRules() ([]network.IngressRule, error) {
	if err := f.initFirewaller(); err != nil {
		return nil, errors.Trace(err)
	}
	return f.fw.IngressRules()
}

func (f *switchingFirewaller) DeleteAllModelGroups() error {
	if err := f.initFirewaller(); err != nil {
		return errors.Trace(err)
	}
	return f.fw.DeleteAllModelGroups()
}

func (f *switchingFirewaller) DeleteAllControllerGroups(controllerUUID string) error {
	if err := f.initFirewaller(); err != nil {
		return errors.Trace(err)
	}
	return f.fw.DeleteAllControllerGroups(controllerUUID)
}

func (f *switchingFirewaller) DeleteGroups(names ...string) error {
	if err := f.initFirewaller(); err != nil {
		return errors.Trace(err)
	}
	return f.fw.DeleteGroups(names...)
}

func (f *switchingFirewaller) GetSecurityGroups(ids ...instance.Id) ([]string, error) {
	if err := f.initFirewaller(); err != nil {
		return nil, errors.Trace(err)
	}
	return f.fw.GetSecurityGroups(ids...)
}

func (f *switchingFirewaller) SetUpGroups(controllerUUID, machineId string, apiPort int) ([]string, error) {
	if err := f.initFirewaller(); err != nil {
		return nil, errors.Trace(err)
	}
	return f.fw.SetUpGroups(controllerUUID, machineId, apiPort)
}

func (f *switchingFirewaller) OpenInstancePorts(inst instance.Instance, machineId string, rules []network.IngressRule) error {
	if err := f.initFirewaller(); err != nil {
		return errors.Trace(err)
	}
	return f.fw.OpenInstancePorts(inst, machineId, rules)
}

func (f *switchingFirewaller) CloseInstancePorts(inst instance.Instance, machineId string, rules []network.IngressRule) error {
	if err := f.initFirewaller(); err != nil {
		return errors.Trace(err)
	}
	return f.fw.CloseInstancePorts(inst, machineId, rules)
}

func (f *switchingFirewaller) InstanceIngressRules(inst instance.Instance, machineId string) ([]network.IngressRule, error) {
	if err := f.initFirewaller(); err != nil {
		return nil, errors.Trace(err)
	}
	return f.fw.InstanceIngressRules(inst, machineId)
}

type firewallerBase struct {
	environ *Environ
}

// GetSecurityGroups implements Firewaller interface.
func (c *firewallerBase) GetSecurityGroups(ids ...instance.Id) ([]string, error) {
	var securityGroupNames []string
	if c.environ.Config().FirewallMode() == config.FwInstance {
		instances, err := c.environ.Instances(ids)
		if err != nil {
			return nil, errors.Trace(err)
		}
		novaClient := c.environ.nova()
		securityGroupNames = make([]string, 0, len(ids))
		for _, inst := range instances {
			if inst == nil {
				continue
			}
			serverId, err := instServerId(inst)
			if err != nil {
				return nil, errors.Trace(err)
			}
			groups, err := novaClient.GetServerSecurityGroups(string(inst.Id()))
			if err != nil {
				return nil, errors.Trace(err)
			}
			for _, group := range groups {
				// We only include the group specifically tied to the instance, not
				// any group global to the model itself.
				suffix := fmt.Sprintf("%s-%s", c.environ.Config().UUID(), serverId)
				if strings.HasSuffix(group.Name, suffix) {
					securityGroupNames = append(securityGroupNames, group.Name)
				}
			}
		}
	}
	return securityGroupNames, nil
}

func instServerId(inst instance.Instance) (string, error) {
	openstackName := inst.(*openstackInstance).getServerDetail().Name
	lastDashPos := strings.LastIndex(openstackName, "-")
	if lastDashPos == -1 {
		return "", errors.Errorf("cannot identify machine ID in openstack server name %q", openstackName)
	}
	serverId := openstackName[lastDashPos+1:]
	return serverId, nil
}

func deleteSecurityGroupsMatchingName(
	deleteSecurityGroups func(match func(name string) bool) error,
	prefix string,
) error {
	re, err := regexp.Compile("^" + prefix)
	if err != nil {
		return errors.Trace(err)
	}
	return deleteSecurityGroups(re.MatchString)
}

func deleteSecurityGroupsOneOfNames(
	deleteSecurityGroups func(match func(name string) bool) error,
	names ...string,
) error {
	match := func(check string) bool {
		for _, name := range names {
			if check == name {
				return true
			}
		}
		return false
	}
	return deleteSecurityGroups(match)
}

// deleteSecurityGroup attempts to delete the security group. Should it fail,
// the deletion is retried due to timing issues in openstack. A security group
// cannot be deleted while it is in use. Theoretically we terminate all the
// instances before we attempt to delete the associated security groups, but
// in practice neutron hasn't always finished with the instance before it
// returns, so there is a race condition where we think the instance is
// terminated and hence attempt to delete the security groups but nova still
// has it around internally. To attempt to catch this timing issue, deletion
// of the groups is tried multiple times.
func deleteSecurityGroup(
	deleteSecurityGroupById func(string) error,
	name, id string,
	clock clock.Clock,
) {
	logger.Debugf("deleting security group %q", name)
	err := retry.Call(retry.CallArgs{
		Func: func() error {
			return deleteSecurityGroupById(id)
		},
		NotifyFunc: func(err error, attempt int) {
			if attempt%4 == 0 {
				message := fmt.Sprintf("waiting to delete security group %q", name)
				if attempt != 4 {
					message = "still " + message
				}
				logger.Debugf(message)
			}
		},
		Attempts: 30,
		Delay:    time.Second,
		Clock:    clock,
	})
	if err != nil {
		logger.Warningf("cannot delete security group %q. Used by another model?", name)
	}
}

func (c *firewallerBase) openPorts(
	openPortsInGroup func(string, []network.IngressRule) error,
	rules []network.IngressRule,
) error {
	if c.environ.Config().FirewallMode() != config.FwGlobal {
		return errors.Errorf("invalid firewall mode %q for opening ports on model",
			c.environ.Config().FirewallMode())
	}
	if err := openPortsInGroup(c.globalGroupRegexp(), rules); err != nil {
		return errors.Trace(err)
	}
	logger.Infof("opened ports in global group: %v", rules)
	return nil
}

func (c *firewallerBase) closePorts(
	closePortsInGroup func(string, []network.IngressRule) error,
	rules []network.IngressRule,
) error {
	if c.environ.Config().FirewallMode() != config.FwGlobal {
		return errors.Errorf("invalid firewall mode %q for closing ports on model",
			c.environ.Config().FirewallMode())
	}
	if err := closePortsInGroup(c.globalGroupRegexp(), rules); err != nil {
		return errors.Trace(err)
	}
	logger.Infof("closed ports in global group: %v", rules)
	return nil
}

func (c *firewallerBase) ingressRules(
	ingressRulesInGroup func(string) ([]network.IngressRule, error),
) ([]network.IngressRule, error) {
	if c.environ.Config().FirewallMode() != config.FwGlobal {
		return nil, errors.Errorf("invalid firewall mode %q for retrieving ingress rules from model",
			c.environ.Config().FirewallMode())
	}
	return ingressRulesInGroup(c.globalGroupRegexp())
}

func (c *firewallerBase) openInstancePorts(
	openPortsInGroup func(string, []network.IngressRule) error,
	machineId string,
	rules []network.IngressRule,
) error {
	if c.environ.Config().FirewallMode() != config.FwInstance {
		return errors.Errorf("invalid firewall mode %q for opening ports on instance",
			c.environ.Config().FirewallMode())
	}
	nameRegexp := c.machineGroupRegexp(machineId)
	if err := openPortsInGroup(nameRegexp, rules); err != nil {
		return errors.Trace(err)
	}
	logger.Infof("opened ports in security group %s-%s: %v", c.environ.Config().UUID(), machineId, rules)
	return nil
}

func (c *firewallerBase) closeInstancePorts(
	closePortsInGroup func(string, []network.IngressRule) error,
	machineId string,
	rules []network.IngressRule,
) error {
	if c.environ.Config().FirewallMode() != config.FwInstance {
		return errors.Errorf("invalid firewall mode %q for closing ports on instance",
			c.environ.Config().FirewallMode())
	}
	nameRegexp := c.machineGroupRegexp(machineId)
	if err := closePortsInGroup(nameRegexp, rules); err != nil {
		return errors.Trace(err)
	}
	logger.Infof("closed ports in security group %s-%s: %v", c.environ.Config().UUID(), machineId, rules)
	return nil
}

func (c *firewallerBase) instanceIngressRules(
	ingressRulesInGroup func(string) ([]network.IngressRule, error),
	machineId string,
) ([]network.IngressRule, error) {
	if c.environ.Config().FirewallMode() != config.FwInstance {
		return nil, errors.Errorf("invalid firewall mode %q for retrieving ingress rules from instance",
			c.environ.Config().FirewallMode())
	}
	nameRegexp := c.machineGroupRegexp(machineId)
	portRanges, err := ingressRulesInGroup(nameRegexp)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return portRanges, nil
}

func (c *firewallerBase) globalGroupName(controllerUUID string) string {
	return fmt.Sprintf("%s-global", c.jujuGroupName(controllerUUID))
}

func (c *firewallerBase) machineGroupName(controllerUUID, machineId string) string {
	return fmt.Sprintf("%s-%s", c.jujuGroupName(controllerUUID), machineId)
}

func (c *firewallerBase) jujuGroupName(controllerUUID string) string {
	cfg := c.environ.Config()
	return fmt.Sprintf("juju-%v-%v", controllerUUID, cfg.UUID())
}

func (c *firewallerBase) jujuControllerGroupPrefix(controllerUUID string) string {
	return fmt.Sprintf("juju-%v-", controllerUUID)
}

func (c *firewallerBase) jujuGroupRegexp() string {
	cfg := c.environ.Config()
	return fmt.Sprintf("juju-.*-%v", cfg.UUID())
}

func (c *firewallerBase) globalGroupRegexp() string {
	return fmt.Sprintf("%s-global", c.jujuGroupRegexp())
}

func (c *firewallerBase) machineGroupRegexp(machineId string) string {
	return fmt.Sprintf("%s-%s", c.jujuGroupRegexp(), machineId)
}

type neutronFirewaller struct {
	firewallerBase
}

// SetUpGroups creates the security groups for the new machine, and
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
func (c *neutronFirewaller) SetUpGroups(controllerUUID, machineId string, apiPort int) ([]string, error) {
	jujuGroup, err := c.setUpGlobalGroup(c.jujuGroupName(controllerUUID), apiPort)
	if err != nil {
		return nil, errors.Trace(err)
	}
	var machineGroup neutron.SecurityGroupV2
	switch c.environ.Config().FirewallMode() {
	case config.FwInstance:
		machineGroup, err = c.ensureGroup(c.machineGroupName(controllerUUID, machineId), nil)
	case config.FwGlobal:
		machineGroup, err = c.ensureGroup(c.globalGroupName(controllerUUID), nil)
	}
	if err != nil {
		return nil, errors.Trace(err)
	}
	groups := []string{jujuGroup.Name, machineGroup.Name}
	if c.environ.ecfg().useDefaultSecurityGroup() {
		groups = append(groups, "default")
	}
	return groups, nil
}

func (c *neutronFirewaller) setUpGlobalGroup(groupName string, apiPort int) (neutron.SecurityGroupV2, error) {
	return c.ensureGroup(groupName,
		[]neutron.RuleInfoV2{
			{
				Direction:      "ingress",
				IPProtocol:     "tcp",
				PortRangeMax:   22,
				PortRangeMin:   22,
				RemoteIPPrefix: "::/0",
				EthernetType:   "IPv6",
			},
			{
				Direction:      "ingress",
				IPProtocol:     "tcp",
				PortRangeMax:   22,
				PortRangeMin:   22,
				RemoteIPPrefix: "0.0.0.0/0",
			},
			{
				Direction:      "ingress",
				IPProtocol:     "tcp",
				PortRangeMax:   apiPort,
				PortRangeMin:   apiPort,
				RemoteIPPrefix: "::/0",
				EthernetType:   "IPv6",
			},
			{
				Direction:      "ingress",
				IPProtocol:     "tcp",
				PortRangeMax:   apiPort,
				PortRangeMin:   apiPort,
				RemoteIPPrefix: "0.0.0.0/0",
			},
			{
				Direction:    "ingress",
				IPProtocol:   "tcp",
				PortRangeMin: 1,
				PortRangeMax: 65535,
				EthernetType: "IPv6",
			},
			{
				Direction:    "ingress",
				IPProtocol:   "tcp",
				PortRangeMin: 1,
				PortRangeMax: 65535,
			},
			{
				Direction:    "ingress",
				IPProtocol:   "udp",
				PortRangeMin: 1,
				PortRangeMax: 65535,
				EthernetType: "IPv6",
			},
			{
				Direction:    "ingress",
				IPProtocol:   "udp",
				PortRangeMin: 1,
				PortRangeMax: 65535,
			},
			{
				Direction:    "ingress",
				IPProtocol:   "icmp",
				EthernetType: "IPv6",
			},
			{
				Direction:  "ingress",
				IPProtocol: "icmp",
			},
		})
}

// zeroGroup holds the zero security group.
var zeroGroup neutron.SecurityGroupV2

// ensureGroup returns the security group with name and perms.
// If a group with name does not exist, one will be created.
// If it exists, its permissions are set to perms.
func (c *neutronFirewaller) ensureGroup(name string, rules []neutron.RuleInfoV2) (neutron.SecurityGroupV2, error) {
	neutronClient := c.environ.neutron()
	// First attempt to look up an existing group by name.
	groupsFound, err := neutronClient.SecurityGroupByNameV2(name)
	if err == nil {
		for _, group := range groupsFound {
			if c.verifyGroupRules(group.Rules, rules) {
				return group, nil
			}
		}
	}
	// Doesn't exist, so try and create it.
	newGroup, err := neutronClient.CreateSecurityGroupV2(name, "juju group")
	if err != nil {
		return zeroGroup, err
	}
	// The new group is created so now add the rules.
	for _, rule := range rules {
		rule.ParentGroupId = newGroup.Id
		// Neutron translates empty RemoteIPPrefix into
		// 0.0.0.0/0 or ::/0 instead of ParentGroupId
		// when EthernetType is set
		if rule.RemoteIPPrefix == "" {
			rule.RemoteGroupId = newGroup.Id
		}
		groupRule, err := neutronClient.CreateSecurityGroupRuleV2(rule)
		if err != nil {
			return zeroGroup, err
		}
		newGroup.Rules = append(newGroup.Rules, *groupRule)
	}
	return *newGroup, nil
}

func countIngressRules(rules []neutron.SecurityGroupRuleV2) int {
	count := 0
	for _, rule := range rules {
		if rule.Direction == "ingress" {
			count += 1
		}
	}
	return count
}

// verifyGroupRules verifies the group rules against the rules we're looking for.
func (c *neutronFirewaller) verifyGroupRules(rules []neutron.SecurityGroupRuleV2, rulesToMatch []neutron.RuleInfoV2) bool {
	if countIngressRules(rules) != len(rulesToMatch) {
		return false
	}
	count := len(rulesToMatch)
	for _, rule := range rules {
		// This is one of the default rules created when a new
		// Neutron Security Group is created
		if rule.Direction == "egress" {
			continue
		}
		for _, toMatch := range rulesToMatch {
			var maxInt int
			if rule.PortRangeMax != nil {
				maxInt = *rule.PortRangeMax
			} else {
				maxInt = 0
			}
			var minInt int
			if rule.PortRangeMin != nil {
				minInt = *rule.PortRangeMin
			} else {
				minInt = 0
			}
			if rule.Direction == toMatch.Direction &&
				rule.RemoteIPPrefix == toMatch.RemoteIPPrefix &&
				*rule.IPProtocol == toMatch.IPProtocol &&
				minInt == toMatch.PortRangeMin &&
				maxInt == toMatch.PortRangeMax {
				count -= 1
				break
			}
		}
	}
	if count != 0 {
		return false
	}
	return true
}

func (c *neutronFirewaller) deleteSecurityGroups(match func(name string) bool) error {
	neutronClient := c.environ.neutron()
	securityGroups, err := neutronClient.ListSecurityGroupsV2()
	if err != nil {
		return errors.Annotate(err, "cannot list security groups")
	}
	for _, group := range securityGroups {
		if match(group.Name) {
			deleteSecurityGroup(
				neutronClient.DeleteSecurityGroupV2,
				group.Name,
				group.Id,
				clock.WallClock,
			)
		}
	}
	return nil
}

// DeleteGroups implements Firewaller interface.
func (c *neutronFirewaller) DeleteGroups(names ...string) error {
	return deleteSecurityGroupsOneOfNames(c.deleteSecurityGroups, names...)
}

// DeleteAllControllerGroups implements Firewaller interface.
func (c *neutronFirewaller) DeleteAllControllerGroups(controllerUUID string) error {
	return deleteSecurityGroupsMatchingName(c.deleteSecurityGroups, c.jujuControllerGroupPrefix(controllerUUID))
}

// DeleteAllModelGroups implements Firewaller interface.
func (c *neutronFirewaller) DeleteAllModelGroups() error {
	return deleteSecurityGroupsMatchingName(c.deleteSecurityGroups, c.jujuGroupRegexp())
}

// OpenPorts implements Firewaller interface.
func (c *neutronFirewaller) OpenPorts(rules []network.IngressRule) error {
	return c.openPorts(c.openPortsInGroup, rules)
}

// ClosePorts implements Firewaller interface.
func (c *neutronFirewaller) ClosePorts(rules []network.IngressRule) error {
	return c.closePorts(c.closePortsInGroup, rules)
}

// IngressRules implements Firewaller interface.
func (c *neutronFirewaller) IngressRules() ([]network.IngressRule, error) {
	return c.ingressRules(c.ingressRulesInGroup)
}

// OpenInstancePorts implements Firewaller interface.
func (c *neutronFirewaller) OpenInstancePorts(inst instance.Instance, machineId string, ports []network.IngressRule) error {
	return c.openInstancePorts(c.openPortsInGroup, machineId, ports)
}

// CloseInstancePorts implements Firewaller interface.
func (c *neutronFirewaller) CloseInstancePorts(inst instance.Instance, machineId string, ports []network.IngressRule) error {
	return c.closeInstancePorts(c.closePortsInGroup, machineId, ports)
}

// InstanceIngressRules implements Firewaller interface.
func (c *neutronFirewaller) InstanceIngressRules(inst instance.Instance, machineId string) ([]network.IngressRule, error) {
	return c.instanceIngressRules(c.ingressRulesInGroup, machineId)
}

// Matching a security group by name only works if each name is unqiue.  Neutron
// security groups are not required to have unique names.  Juju constructs unique
// names, but there are frequently multiple matches to 'default'
func (c *neutronFirewaller) matchingGroup(nameRegExp string) (neutron.SecurityGroupV2, error) {
	re, err := regexp.Compile(nameRegExp)
	if err != nil {
		return neutron.SecurityGroupV2{}, err
	}
	neutronClient := c.environ.neutron()
	allGroups, err := neutronClient.ListSecurityGroupsV2()
	if err != nil {
		return neutron.SecurityGroupV2{}, err
	}
	var matchingGroups []neutron.SecurityGroupV2
	for _, group := range allGroups {
		if re.MatchString(group.Name) {
			matchingGroups = append(matchingGroups, group)
		}
	}
	if len(matchingGroups) != 1 {
		return neutron.SecurityGroupV2{}, errors.NotFoundf("security groups matching %q", nameRegExp)
	}
	return matchingGroups[0], nil
}

func (c *neutronFirewaller) openPortsInGroup(nameRegExp string, rules []network.IngressRule) error {
	group, err := c.matchingGroup(nameRegExp)
	if err != nil {
		return errors.Trace(err)
	}
	neutronClient := c.environ.neutron()
	ruleInfo := rulesToRuleInfo(group.Id, rules)
	for _, rule := range ruleInfo {
		_, err := neutronClient.CreateSecurityGroupRuleV2(rule)
		if err != nil {
			// TODO: if err is not rule already exists, raise?
			logger.Debugf("error creating security group rule: %v", err.Error())
		}
	}
	return nil
}

// secGroupMatchesIngressRule checks if supplied nova security group rule matches the ingress rule
func secGroupMatchesIngressRule(secGroupRule neutron.SecurityGroupRuleV2, rule network.IngressRule) bool {
	if secGroupRule.IPProtocol == nil || *secGroupRule.PortRangeMax == 0 || *secGroupRule.PortRangeMin == 0 {
		return false
	}
	portsMatch := *secGroupRule.IPProtocol == rule.Protocol &&
		*secGroupRule.PortRangeMin == rule.FromPort &&
		*secGroupRule.PortRangeMax == rule.ToPort
	if !portsMatch {
		return false
	}
	// The ports match, so if the security group RemoteIPPrefix matches *any* of the
	// rule's source ranges, then that's a match.
	if len(rule.SourceCIDRs) == 0 {
		return secGroupRule.RemoteIPPrefix == "" || secGroupRule.RemoteIPPrefix == "0.0.0.0/0"
	}
	for _, r := range rule.SourceCIDRs {
		if r == secGroupRule.RemoteIPPrefix {
			return true
		}
	}
	return false
}

func (c *neutronFirewaller) closePortsInGroup(nameRegExp string, rules []network.IngressRule) error {
	if len(rules) == 0 {
		return nil
	}
	group, err := c.matchingGroup(nameRegExp)
	if err != nil {
		return errors.Trace(err)
	}
	neutronClient := c.environ.neutron()
	// TODO: Hey look ma, it's quadratic
	for _, rule := range rules {
		for _, p := range group.Rules {
			if !secGroupMatchesIngressRule(p, rule) {
				continue
			}
			err := neutronClient.DeleteSecurityGroupRuleV2(p.Id)
			if err != nil {
				return errors.Trace(err)
			}
			break
		}
	}
	return nil
}

func (c *neutronFirewaller) ingressRulesInGroup(nameRegexp string) (rules []network.IngressRule, err error) {
	group, err := c.matchingGroup(nameRegexp)
	if err != nil {
		return nil, errors.Trace(err)
	}
	// Keep track of all the RemoteIPPrefixes for each port range.
	portSourceCIDRs := make(map[network.PortRange]*[]string)
	for _, p := range group.Rules {
		// Skip the default Security Group Rules created by Neutron
		if p.Direction == "egress" {
			continue
		}
		portRange := network.PortRange{
			Protocol: *p.IPProtocol,
		}
		if p.PortRangeMin != nil {
			portRange.FromPort = *p.PortRangeMin
		}
		if p.PortRangeMax != nil {
			portRange.ToPort = *p.PortRangeMax
		}
		// Record the RemoteIPPrefix for the port range.
		remotePrefix := p.RemoteIPPrefix
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
			return nil, errors.Trace(err)
		}
		rules = append(rules, rule)
	}
	network.SortIngressRules(rules)
	return rules, nil
}
