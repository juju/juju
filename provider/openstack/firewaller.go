// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package openstack

import (
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/juju/errors"
	"github.com/juju/retry"
	"github.com/juju/utils/clock"
	gooseerrors "gopkg.in/goose.v1/errors"
	"gopkg.in/goose.v1/nova"

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
	OpenPorts(ports []network.PortRange) error

	// ClosePorts closes the given port ranges for the whole environment.
	ClosePorts(ports []network.PortRange) error

	// Ports returns the port ranges opened for the whole environment.
	Ports() ([]network.PortRange, error)

	// Implementations shoud delete all global security groups.
	DeleteGlobalGroups() error

	// Implementations should return list of security groups, that belong to given instances.
	GetSecurityGroups(ids ...instance.Id) ([]string, error)

	// Implementations should set up initial security groups, if any.
	SetUpGroups(machineId string, apiPort int) ([]nova.SecurityGroup, error)

	// Set of initial networks, that should be added by default to all new instances.
	InitialNetworks() []nova.ServerNetworks

	// OpenInstancePorts opens the given port ranges for the specified  instance.
	OpenInstancePorts(inst instance.Instance, machineId string, ports []network.PortRange) error

	// CloseInstancePorts closes the given port ranges for the specified  instance.
	CloseInstancePorts(inst instance.Instance, machineId string, ports []network.PortRange) error

	// InstancePorts returns the port ranges opened for the specified  instance.
	InstancePorts(inst instance.Instance, machineId string) ([]network.PortRange, error)
}

type firewallerFactory struct {
}

// GetFirewaller implements FirewallerFactory
func (f *firewallerFactory) GetFirewaller(env environs.Environ) Firewaller {
	return &defaultFirewaller{environ: env.(*Environ)}
}

type defaultFirewaller struct {
	environ *Environ
}

// InitialNetworks implements Firewaller interface.
func (c *defaultFirewaller) InitialNetworks() []nova.ServerNetworks {
	return []nova.ServerNetworks{}
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
func (c *defaultFirewaller) SetUpGroups(machineId string, apiPort int) ([]nova.SecurityGroup, error) {
	jujuGroup, err := c.setUpGlobalGroup(c.jujuGroupName(), apiPort)
	if err != nil {
		return nil, err
	}
	var machineGroup nova.SecurityGroup
	switch c.environ.Config().FirewallMode() {
	case config.FwInstance:
		machineGroup, err = c.ensureGroup(c.machineGroupName(machineId), nil)
	case config.FwGlobal:
		machineGroup, err = c.ensureGroup(c.globalGroupName(), nil)
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

func (c *defaultFirewaller) setUpGlobalGroup(groupName string, apiPort int) (nova.SecurityGroup, error) {
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
func (c *defaultFirewaller) ensureGroup(name string, rules []nova.RuleInfo) (nova.SecurityGroup, error) {
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

// GetSecurityGroups implements Firewaller interface.
func (c *defaultFirewaller) GetSecurityGroups(ids ...instance.Id) ([]string, error) {
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
			securityGroupName := c.machineGroupName(openstackName[lastDashPos+1:])
			securityGroupNames = append(securityGroupNames, securityGroupName)
		}
	}
	return securityGroupNames, nil
}

// DeleteGlobalGroups implements Firewaller interface.
func (c *defaultFirewaller) DeleteGlobalGroups() error {
	novaClient := c.environ.nova()
	securityGroups, err := novaClient.ListSecurityGroups()
	if err != nil {
		return errors.Annotate(err, "cannot list security groups")
	}
	re, err := regexp.Compile(fmt.Sprintf("^%s(-\\d+)?$", c.jujuGroupName()))
	if err != nil {
		return errors.Trace(err)
	}
	globalGroupName := c.globalGroupName()
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
	logger.Debugf("deleting security group %q", name)
	err := retry.Call(retry.CallArgs{
		Func: func() error {
			return novaclient.DeleteSecurityGroup(id)
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
		Clock:    clock.WallClock,
	})
	if err != nil {
		logger.Warningf("cannot delete security group %q. Used by another model?", name)
	}
}

// OpenPorts implements Firewaller interface.
func (c *defaultFirewaller) OpenPorts(ports []network.PortRange) error {
	if c.environ.Config().FirewallMode() != config.FwGlobal {
		return fmt.Errorf("invalid firewall mode %q for opening ports on model",
			c.environ.Config().FirewallMode())
	}
	if err := c.openPortsInGroup(c.globalGroupName(), ports); err != nil {
		return err
	}
	logger.Infof("opened ports in global group: %v", ports)
	return nil
}

// ClosePorts implements Firewaller interface.
func (c *defaultFirewaller) ClosePorts(ports []network.PortRange) error {
	if c.environ.Config().FirewallMode() != config.FwGlobal {
		return fmt.Errorf("invalid firewall mode %q for closing ports on model",
			c.environ.Config().FirewallMode())
	}
	if err := c.closePortsInGroup(c.globalGroupName(), ports); err != nil {
		return err
	}
	logger.Infof("closed ports in global group: %v", ports)
	return nil
}

// Ports implements Firewaller interface.
func (c *defaultFirewaller) Ports() ([]network.PortRange, error) {
	if c.environ.Config().FirewallMode() != config.FwGlobal {
		return nil, fmt.Errorf("invalid firewall mode %q for retrieving ports from model",
			c.environ.Config().FirewallMode())
	}
	return c.portsInGroup(c.globalGroupName())
}

// OpenInstancePorts implements Firewaller interface.
func (c *defaultFirewaller) OpenInstancePorts(inst instance.Instance, machineId string, ports []network.PortRange) error {
	if c.environ.Config().FirewallMode() != config.FwInstance {
		return fmt.Errorf("invalid firewall mode %q for opening ports on instance",
			c.environ.Config().FirewallMode())
	}
	name := c.machineGroupName(machineId)
	if err := c.openPortsInGroup(name, ports); err != nil {
		return err
	}
	logger.Infof("opened ports in security group %s: %v", name, ports)
	return nil
}

// CloseInstancePorts implements Firewaller interface.
func (c *defaultFirewaller) CloseInstancePorts(inst instance.Instance, machineId string, ports []network.PortRange) error {
	if c.environ.Config().FirewallMode() != config.FwInstance {
		return fmt.Errorf("invalid firewall mode %q for closing ports on instance",
			c.environ.Config().FirewallMode())
	}
	name := c.machineGroupName(machineId)
	if err := c.closePortsInGroup(name, ports); err != nil {
		return err
	}
	logger.Infof("closed ports in security group %s: %v", name, ports)
	return nil
}

// InstancePorts implements Firewaller interface.
func (c *defaultFirewaller) InstancePorts(inst instance.Instance, machineId string) ([]network.PortRange, error) {
	if c.environ.Config().FirewallMode() != config.FwInstance {
		return nil, fmt.Errorf("invalid firewall mode %q for retrieving ports from instance",
			c.environ.Config().FirewallMode())
	}
	name := c.machineGroupName(machineId)
	portRanges, err := c.portsInGroup(name)
	if err != nil {
		return nil, err
	}
	return portRanges, nil
}

func (c *defaultFirewaller) openPortsInGroup(name string, portRanges []network.PortRange) error {
	novaclient := c.environ.nova()
	group, err := novaclient.SecurityGroupByName(name)
	if err != nil {
		return err
	}
	rules := portsToRuleInfo(group.Id, portRanges)
	for _, rule := range rules {
		_, err := novaclient.CreateSecurityGroupRule(rule)
		if err != nil {
			// TODO: if err is not rule already exists, raise?
			logger.Debugf("error creating security group rule: %v", err.Error())
		}
	}
	return nil
}

// ruleMatchesPortRange checks if supplied nova security group rule matches the port range
func ruleMatchesPortRange(rule nova.SecurityGroupRule, portRange network.PortRange) bool {
	if rule.IPProtocol == nil || rule.FromPort == nil || rule.ToPort == nil {
		return false
	}
	return *rule.IPProtocol == portRange.Protocol &&
		*rule.FromPort == portRange.FromPort &&
		*rule.ToPort == portRange.ToPort
}

func (c *defaultFirewaller) closePortsInGroup(name string, portRanges []network.PortRange) error {
	if len(portRanges) == 0 {
		return nil
	}
	novaclient := c.environ.nova()
	group, err := novaclient.SecurityGroupByName(name)
	if err != nil {
		return err
	}
	// TODO: Hey look ma, it's quadratic
	for _, portRange := range portRanges {
		for _, p := range (*group).Rules {
			if !ruleMatchesPortRange(p, portRange) {
				continue
			}
			err := novaclient.DeleteSecurityGroupRule(p.Id)
			if err != nil {
				return err
			}
			break
		}
	}
	return nil
}

func (c *defaultFirewaller) portsInGroup(name string) (portRanges []network.PortRange, err error) {
	group, err := c.environ.nova().SecurityGroupByName(name)
	if err != nil {
		return nil, err
	}
	for _, p := range (*group).Rules {
		portRanges = append(portRanges, network.PortRange{
			Protocol: *p.IPProtocol,
			FromPort: *p.FromPort,
			ToPort:   *p.ToPort,
		})
	}
	network.SortPortRanges(portRanges)
	return portRanges, nil
}
func (c *defaultFirewaller) globalGroupName() string {
	return fmt.Sprintf("%s-global", c.jujuGroupName())
}

func (c *defaultFirewaller) machineGroupName(machineId string) string {
	return fmt.Sprintf("%s-%s", c.jujuGroupName(), machineId)
}

func (c *defaultFirewaller) jujuGroupName() string {
	cfg := c.environ.Config()
	eUUID, _ := cfg.UUID()
	return fmt.Sprintf("juju-%s", eUUID)
}
