// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package openstack

import (
	"fmt"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/juju/clock"
	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"github.com/juju/retry"
	gooseerrors "gopkg.in/goose.v2/errors"
	"gopkg.in/goose.v2/neutron"

	"github.com/juju/juju/core/instance"
	corenetwork "github.com/juju/juju/core/network"
	"github.com/juju/juju/core/network/firewall"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/environs/context"
	"github.com/juju/juju/environs/instances"
	"github.com/juju/juju/provider/common"
)

const (
	validUUID              = `[a-f0-9]{8}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{12}`
	GroupControllerPattern = `^(?P<prefix>juju-)(?P<controllerUUID>` + validUUID + `)(?P<suffix>-.*)$`
)

var extractControllerRe = regexp.MustCompile(GroupControllerPattern)

// FirewallerFactory for obtaining firewaller object.
type FirewallerFactory interface {
	GetFirewaller(env environs.Environ) Firewaller
}

// Firewaller allows custom openstack provider behaviour.
// This is used in other providers that embed the openstack provider.
type Firewaller interface {
	// OpenPorts opens the given port ranges for the whole environment.
	OpenPorts(ctx context.ProviderCallContext, rules firewall.IngressRules) error

	// ClosePorts closes the given port ranges for the whole environment.
	ClosePorts(ctx context.ProviderCallContext, rules firewall.IngressRules) error

	// IngressRules returns the ingress rules applied to the whole environment.
	// It is expected that there be only one ingress rule result for a given
	// port range - the rule's SourceCIDRs will contain all applicable source
	// address rules for that port range.
	IngressRules(ctx context.ProviderCallContext) (firewall.IngressRules, error)

	// DeleteAllModelGroups deletes all security groups for the
	// model.
	DeleteAllModelGroups(ctx context.ProviderCallContext) error

	// DeleteAllControllerGroups deletes all security groups for the
	// controller, ie those for all hosted models.
	DeleteAllControllerGroups(ctx context.ProviderCallContext, controllerUUID string) error

	// DeleteGroups deletes the security groups with the specified names.
	DeleteGroups(ctx context.ProviderCallContext, names ...string) error

	// UpdateGroupController updates all of the security groups for
	// this model to refer to the specified controller, such that
	// DeleteAllControllerGroups will remove them only when called
	// with the specified controller ID.
	UpdateGroupController(ctx context.ProviderCallContext, controllerUUID string) error

	// GetSecurityGroups returns a list of the security groups that
	// belong to given instances.
	GetSecurityGroups(ctx context.ProviderCallContext, ids ...instance.Id) ([]string, error)

	// SetUpGroups sets up initial security groups, if any, and returns
	// their names.
	SetUpGroups(ctx context.ProviderCallContext, controllerUUID, machineID string, apiPort int) ([]string, error)

	// OpenInstancePorts opens the given port ranges for the specified  instance.
	OpenInstancePorts(ctx context.ProviderCallContext, inst instances.Instance, machineID string, rules firewall.IngressRules) error

	// CloseInstancePorts closes the given port ranges for the specified  instance.
	CloseInstancePorts(ctx context.ProviderCallContext, inst instances.Instance, machineID string, rules firewall.IngressRules) error

	// InstanceIngressRules returns the ingress rules applied to the specified  instance.
	InstanceIngressRules(ctx context.ProviderCallContext, inst instances.Instance, machineID string) (firewall.IngressRules, error)
}

type firewallerFactory struct{}

// GetFirewaller implements FirewallerFactory
func (f *firewallerFactory) GetFirewaller(env environs.Environ) Firewaller {
	return &neutronFirewaller{firewallerBase{environ: env.(*Environ)}}
}

type firewallerBase struct {
	environ          *Environ
	ensureGroupMutex sync.Mutex
}

// GetSecurityGroups implements Firewaller interface.
func (c *firewallerBase) GetSecurityGroups(ctx context.ProviderCallContext, ids ...instance.Id) ([]string, error) {
	var securityGroupNames []string
	if c.environ.Config().FirewallMode() == config.FwInstance {
		instances, err := c.environ.Instances(ctx, ids)
		if err != nil {
			return nil, errors.Trace(err)
		}
		novaClient := c.environ.nova()
		securityGroupNames = make([]string, 0, len(ids))
		for _, inst := range instances {
			if inst == nil {
				continue
			}
			serverID, err := instServerID(inst)
			if err != nil {
				return nil, errors.Trace(err)
			}
			groups, err := novaClient.GetServerSecurityGroups(string(inst.Id()))
			if err != nil {
				handleCredentialError(err, ctx)
				return nil, errors.Trace(err)
			}
			for _, group := range groups {
				// We only include the group specifically tied to the instance, not
				// any group global to the model itself.
				suffix := fmt.Sprintf("%s-%s", c.environ.Config().UUID(), serverID)
				if strings.HasSuffix(group.Name, suffix) {
					securityGroupNames = append(securityGroupNames, group.Name)
				}
			}
		}
	}
	return securityGroupNames, nil
}

func instServerID(inst instances.Instance) (string, error) {
	openstackName := inst.(*openstackInstance).getServerDetail().Name
	lastDashPos := strings.LastIndex(openstackName, "-")
	if lastDashPos == -1 {
		return "", errors.Errorf("cannot identify machine ID in openstack server name %q", openstackName)
	}
	return openstackName[lastDashPos+1:], nil
}

func deleteSecurityGroupsMatchingName(
	ctx context.ProviderCallContext,
	deleteSecurityGroups func(ctx context.ProviderCallContext, match func(name string) bool) error,
	prefix string,
) error {
	re, err := regexp.Compile("^" + prefix)
	if err != nil {
		return errors.Trace(err)
	}
	return deleteSecurityGroups(ctx, re.MatchString)
}

func deleteSecurityGroupsOneOfNames(
	ctx context.ProviderCallContext,
	deleteSecurityGroups func(ctx context.ProviderCallContext, match func(name string) bool) error,
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
	return deleteSecurityGroups(ctx, match)
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
	ctx context.ProviderCallContext,
	deleteSecurityGroupByID func(string) error,
	name, id string,
	clock clock.Clock,
) {
	logger.Debugf("deleting security group %q", name)
	err := retry.Call(retry.CallArgs{
		Func: func() error {
			if err := deleteSecurityGroupByID(id); err != nil {
				if gooseerrors.IsNotFound(err) {
					return nil
				}
				handleCredentialError(err, ctx)
				return errors.Trace(err)
			}
			return nil
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
	ctx context.ProviderCallContext,
	openPortsInGroup func(context.ProviderCallContext, string, firewall.IngressRules) error,
	rules firewall.IngressRules,
) error {
	if c.environ.Config().FirewallMode() != config.FwGlobal {
		return errors.Errorf("invalid firewall mode %q for opening ports on model",
			c.environ.Config().FirewallMode())
	}
	if err := openPortsInGroup(ctx, c.globalGroupRegexp(), rules); err != nil {
		return errors.Trace(err)
	}
	logger.Infof("opened ports in global group: %v", rules)
	return nil
}

func (c *firewallerBase) closePorts(
	ctx context.ProviderCallContext,
	closePortsInGroup func(context.ProviderCallContext, string, firewall.IngressRules) error,
	rules firewall.IngressRules,
) error {
	if c.environ.Config().FirewallMode() != config.FwGlobal {
		return errors.Errorf("invalid firewall mode %q for closing ports on model",
			c.environ.Config().FirewallMode())
	}
	if err := closePortsInGroup(ctx, c.globalGroupRegexp(), rules); err != nil {
		return errors.Trace(err)
	}
	logger.Infof("closed ports in global group: %v", rules)
	return nil
}

func (c *firewallerBase) ingressRules(
	ctx context.ProviderCallContext,
	ingressRulesInGroup func(context.ProviderCallContext, string) (firewall.IngressRules, error),
) (firewall.IngressRules, error) {
	if c.environ.Config().FirewallMode() != config.FwGlobal {
		return nil, errors.Errorf("invalid firewall mode %q for retrieving ingress rules from model",
			c.environ.Config().FirewallMode())
	}
	return ingressRulesInGroup(ctx, c.globalGroupRegexp())
}

func (c *firewallerBase) openInstancePorts(
	ctx context.ProviderCallContext,
	openPortsInGroup func(context.ProviderCallContext, string, firewall.IngressRules) error,
	machineID string,
	rules firewall.IngressRules,
) error {
	nameRegexp := c.machineGroupRegexp(machineID)
	if err := openPortsInGroup(ctx, nameRegexp, rules); err != nil {
		return errors.Trace(err)
	}
	logger.Infof("opened ports in security group %s-%s: %v", c.environ.Config().UUID(), machineID, rules)
	return nil
}

func (c *firewallerBase) closeInstancePorts(
	ctx context.ProviderCallContext,
	closePortsInGroup func(context.ProviderCallContext, string, firewall.IngressRules) error,
	machineID string,
	rules firewall.IngressRules,
) error {
	nameRegexp := c.machineGroupRegexp(machineID)
	if err := closePortsInGroup(ctx, nameRegexp, rules); err != nil {
		return errors.Trace(err)
	}
	logger.Infof("closed ports in security group %s-%s: %v", c.environ.Config().UUID(), machineID, rules)
	return nil
}

func (c *firewallerBase) instanceIngressRules(
	ctx context.ProviderCallContext,
	ingressRulesInGroup func(context.ProviderCallContext, string) (firewall.IngressRules, error),
	machineID string,
) (firewall.IngressRules, error) {
	nameRegexp := c.machineGroupRegexp(machineID)
	portRanges, err := ingressRulesInGroup(ctx, nameRegexp)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return portRanges, nil
}

func (c *firewallerBase) globalGroupName(controllerUUID string) string {
	return fmt.Sprintf("%s-global", c.jujuGroupName(controllerUUID))
}

func (c *firewallerBase) machineGroupName(controllerUUID, machineID string) string {
	return fmt.Sprintf("%s-%s", c.jujuGroupName(controllerUUID), machineID)
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

func (c *firewallerBase) machineGroupRegexp(machineID string) string {
	// we are only looking to match 1 machine
	return fmt.Sprintf("%s-%s$", c.jujuGroupRegexp(), machineID)
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
func (c *neutronFirewaller) SetUpGroups(ctx context.ProviderCallContext, controllerUUID, machineID string, apiPort int) ([]string, error) {
	jujuGroup, err := c.setUpGlobalGroup(c.jujuGroupName(controllerUUID), apiPort)
	if err != nil {
		return nil, errors.Trace(err)
	}
	var machineGroup neutron.SecurityGroupV2
	switch c.environ.Config().FirewallMode() {
	case config.FwInstance:
		machineGroup, err = c.ensureGroup(c.machineGroupName(controllerUUID, machineID), nil)
	case config.FwGlobal:
		machineGroup, err = c.ensureGroup(c.globalGroupName(controllerUUID), nil)
	}
	if err != nil {
		handleCredentialError(err, ctx)
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

// ensureGroup returns the security group with name and rules.
// If a group with name does not exist, one will be created.
// If it exists, its permissions are set to rules.
func (c *neutronFirewaller) ensureGroup(name string, rules []neutron.RuleInfoV2) (neutron.SecurityGroupV2, error) {
	// Due to parallelization of the provisioner, it's possible that we try
	// to create the model security group a second time before the first time
	// is complete causing failures.
	// TODO (stickupkid): This can block forever (API timeouts). We should allow
	// a mutex to timeout and fail with an error.
	c.ensureGroupMutex.Lock()
	defer c.ensureGroupMutex.Unlock()

	neutronClient := c.environ.neutron()
	var group neutron.SecurityGroupV2

	// First attempt to look up an existing group by name.
	groupsFound, err := neutronClient.SecurityGroupByNameV2(name)
	// a list is returned, but there should be only one
	if err == nil && len(groupsFound) == 1 {
		group = groupsFound[0]
	} else if err != nil && strings.Contains(err.Error(), "failed to find security group") {
		// TODO(hml): We should use a typed error here.  SecurityGroupByNameV2
		// doesn't currently return one for this case.
		g, err := neutronClient.CreateSecurityGroupV2(name, "juju group")
		if err != nil {
			return zeroGroup, err
		}
		group = *g
	} else if err == nil && len(groupsFound) > 1 {
		// TODO(hml): Add unit test for this case
		return zeroGroup, errors.New(fmt.Sprintf("More than one security group named %s was found", name))
	} else {
		return zeroGroup, err
	}

	have := newRuleInfoSetFromRules(group.Rules)
	want := newRuleInfoSetFromRuleInfo(rules)

	// Find rules we want to delete, that we have but don't want, and
	// delete them.
	// Define a removal set to ensure that we only ever delete a ruleID once.
	remove := set.NewStrings()
	for k := range have {
		// Neutron creates 2 egress rules with any new Security Group.
		// Keep them.
		if _, ok := want[k]; !ok && k.Direction != "egress" {
			remove.Add(have[k])
		}
	}
	for _, ruleID := range remove.SortedValues() {
		if err = neutronClient.DeleteSecurityGroupRuleV2(ruleID); err != nil {
			if gooseerrors.IsNotFound(err) {
				continue
			}
			return zeroGroup, err
		}
	}

	// Find rules we want to add, that we want but don't have, and add
	// them.
	add := make(ruleInfoSet)
	for k := range want {
		if _, ok := have[k]; !ok {
			add[k] = want[k]
		}
	}
	for rule := range add {
		rule.ParentGroupId = group.Id
		// Neutron translates empty RemoteIPPrefix into
		// 0.0.0.0/0 or ::/0 instead of ParentGroupId
		// when EthernetType is set
		if rule.RemoteIPPrefix == "" {
			rule.RemoteGroupId = group.Id
		}
		if _, err := neutronClient.CreateSecurityGroupRuleV2(rule); err != nil {
			return zeroGroup, err
		}
	}

	// Since we may have done a few add or delete rules, get a new
	// copy of the security group to return containing the end
	// list of rules.
	groupsFound, err = neutronClient.SecurityGroupByNameV2(name)
	if err != nil {
		return zeroGroup, err
	} else if len(groupsFound) > 1 {
		// TODO(hml): Add unit test for this case
		return zeroGroup, errors.New(fmt.Sprintf("More than one security group named %s was found after group was ensured", name))
	}
	return groupsFound[0], nil
}

// ruleInfoSet represents a Security Group Rule created for a Security Group.
// The string will be the Security Group Rule Id, if the rule has previously been
// created.
type ruleInfoSet map[neutron.RuleInfoV2]string

// newRuleSetForGroup returns a set of all of the permissions in a given
// slice of SecurityGroupRules.  It ignores the group id, the
// remove group id, and tenant id.  Keep the rule id to delete the rule if
// necessary.
func newRuleInfoSetFromRules(rules []neutron.SecurityGroupRuleV2) ruleInfoSet {
	m := make(ruleInfoSet)
	for _, r := range rules {
		k := neutron.RuleInfoV2{
			Direction:      r.Direction,
			EthernetType:   r.EthernetType,
			RemoteIPPrefix: r.RemoteIPPrefix,
		}
		if r.IPProtocol != nil {
			k.IPProtocol = *r.IPProtocol
		}
		if r.PortRangeMax != nil {
			k.PortRangeMax = *r.PortRangeMax
		}
		if r.PortRangeMin != nil {
			k.PortRangeMin = *r.PortRangeMin
		}
		m[k] = r.Id
	}
	return m
}

// newRuleSetForGroup returns a set of all of the permissions in a given
// slice of RuleInfo.  It ignores the rule id, the group id, the
// remove group id, and tenant id.
func newRuleInfoSetFromRuleInfo(rules []neutron.RuleInfoV2) ruleInfoSet {
	m := make(ruleInfoSet)
	for _, r := range rules {
		k := neutron.RuleInfoV2{
			Direction:      r.Direction,
			IPProtocol:     r.IPProtocol,
			PortRangeMin:   r.PortRangeMin,
			PortRangeMax:   r.PortRangeMax,
			EthernetType:   r.EthernetType,
			RemoteIPPrefix: r.RemoteIPPrefix,
		}
		m[k] = ""
	}
	return m
}

func (c *neutronFirewaller) deleteSecurityGroups(ctx context.ProviderCallContext, match func(name string) bool) error {
	neutronClient := c.environ.neutron()
	securityGroups, err := neutronClient.ListSecurityGroupsV2()
	if err != nil {
		handleCredentialError(err, ctx)
		return errors.Annotate(err, "cannot list security groups")
	}
	for _, group := range securityGroups {
		if match(group.Name) {
			deleteSecurityGroup(
				ctx,
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
func (c *neutronFirewaller) DeleteGroups(ctx context.ProviderCallContext, names ...string) error {
	return deleteSecurityGroupsOneOfNames(ctx, c.deleteSecurityGroups, names...)
}

// DeleteAllControllerGroups implements Firewaller interface.
func (c *neutronFirewaller) DeleteAllControllerGroups(ctx context.ProviderCallContext, controllerUUID string) error {
	return deleteSecurityGroupsMatchingName(ctx, c.deleteSecurityGroups, c.jujuControllerGroupPrefix(controllerUUID))
}

// DeleteAllModelGroups implements Firewaller interface.
func (c *neutronFirewaller) DeleteAllModelGroups(ctx context.ProviderCallContext) error {
	return deleteSecurityGroupsMatchingName(ctx, c.deleteSecurityGroups, c.jujuGroupRegexp())
}

// UpdateGroupController implements Firewaller interface.
func (c *neutronFirewaller) UpdateGroupController(ctx context.ProviderCallContext, controllerUUID string) error {
	neutronClient := c.environ.neutron()
	groups, err := neutronClient.ListSecurityGroupsV2()
	if err != nil {
		handleCredentialError(err, ctx)
		return errors.Trace(err)
	}
	re, err := regexp.Compile(c.jujuGroupRegexp())
	if err != nil {
		handleCredentialError(err, ctx)
		return errors.Trace(err)
	}

	var failed []string
	for _, group := range groups {
		if !re.MatchString(group.Name) {
			continue
		}
		err := c.updateGroupControllerUUID(&group, controllerUUID)
		if err != nil {
			logger.Errorf("error updating controller for security group %s: %v", group.Id, err)
			failed = append(failed, group.Id)
			if common.MaybeHandleCredentialError(IsAuthorisationFailure, err, ctx) {
				// No need to continue here since we will 100% fail with an invalid credential.
				break
			}

		}
	}
	if len(failed) != 0 {
		return errors.Errorf("errors updating controller for security groups: %v", failed)
	}
	return nil
}

func (c *neutronFirewaller) updateGroupControllerUUID(group *neutron.SecurityGroupV2, controllerUUID string) error {
	newName, err := replaceControllerUUID(group.Name, controllerUUID)
	if err != nil {
		return errors.Trace(err)
	}
	client := c.environ.neutron()
	_, err = client.UpdateSecurityGroupV2(group.Id, newName, group.Description)
	return errors.Trace(err)
}

// OpenPorts implements Firewaller interface.
func (c *neutronFirewaller) OpenPorts(ctx context.ProviderCallContext, rules firewall.IngressRules) error {
	err := c.openPorts(ctx, c.openPortsInGroup, rules)
	if err != nil {
		handleCredentialError(err, ctx)
		return errors.Trace(err)
	}
	return nil
}

// ClosePorts implements Firewaller interface.
func (c *neutronFirewaller) ClosePorts(ctx context.ProviderCallContext, rules firewall.IngressRules) error {
	err := c.closePorts(ctx, c.closePortsInGroup, rules)
	if err != nil {
		handleCredentialError(err, ctx)
		return errors.Trace(err)
	}
	return nil
}

// IngressRules implements Firewaller interface.
func (c *neutronFirewaller) IngressRules(ctx context.ProviderCallContext) (firewall.IngressRules, error) {
	rules, err := c.ingressRules(ctx, c.ingressRulesInGroup)
	if err != nil {
		handleCredentialError(err, ctx)
		return rules, errors.Trace(err)
	}
	return rules, nil
}

// OpenInstancePorts implements Firewaller interface.
func (c *neutronFirewaller) OpenInstancePorts(ctx context.ProviderCallContext, inst instances.Instance, machineID string, ports firewall.IngressRules) error {
	if c.environ.Config().FirewallMode() != config.FwInstance {
		return errors.Errorf("invalid firewall mode %q for opening ports on instance",
			c.environ.Config().FirewallMode())
	}
	// For bug 1680787
	// No security groups exist if the network used to boot the instance has
	// PortSecurityEnabled set to false.  To avoid filling up the log files,
	// skip trying to open ports in this cases.
	if securityGroups := inst.(*openstackInstance).getServerDetail().Groups; securityGroups == nil {
		return nil
	}
	err := c.openInstancePorts(ctx, c.openPortsInGroup, machineID, ports)
	if err != nil {
		handleCredentialError(err, ctx)
		return errors.Trace(err)
	}
	return nil
}

// CloseInstancePorts implements Firewaller interface.
func (c *neutronFirewaller) CloseInstancePorts(ctx context.ProviderCallContext, inst instances.Instance, machineID string, ports firewall.IngressRules) error {
	if c.environ.Config().FirewallMode() != config.FwInstance {
		return errors.Errorf("invalid firewall mode %q for closing ports on instance",
			c.environ.Config().FirewallMode())
	}
	// For bug 1680787
	// No security groups exist if the network used to boot the instance has
	// PortSecurityEnabled set to false.  To avoid filling up the log files,
	// skip trying to open ports in this cases.
	if securityGroups := inst.(*openstackInstance).getServerDetail().Groups; securityGroups == nil {
		return nil
	}
	err := c.closeInstancePorts(ctx, c.closePortsInGroup, machineID, ports)
	if err != nil {
		handleCredentialError(err, ctx)
		return errors.Trace(err)
	}
	return nil
}

// InstanceIngressRules implements Firewaller interface.
func (c *neutronFirewaller) InstanceIngressRules(ctx context.ProviderCallContext, inst instances.Instance, machineID string) (firewall.IngressRules, error) {
	if c.environ.Config().FirewallMode() != config.FwInstance {
		return nil, errors.Errorf("invalid firewall mode %q for retrieving ingress rules from instance",
			c.environ.Config().FirewallMode())
	}
	// For bug 1680787
	// No security groups exist if the network used to boot the instance has
	// PortSecurityEnabled set to false.  To avoid filling up the log files,
	// skip trying to open ports in this cases.
	if securityGroups := inst.(*openstackInstance).getServerDetail().Groups; securityGroups == nil {
		return firewall.IngressRules{}, nil
	}
	rules, err := c.instanceIngressRules(ctx, c.ingressRulesInGroup, machineID)
	if err != nil {
		handleCredentialError(err, ctx)
		return rules, errors.Trace(err)
	}
	return rules, err
}

// Matching a security group by name only works if each name is unqiue.  Neutron
// security groups are not required to have unique names.  Juju constructs unique
// names, but there are frequently multiple matches to 'default'
func (c *neutronFirewaller) matchingGroup(ctx context.ProviderCallContext, nameRegExp string) (neutron.SecurityGroupV2, error) {
	re, err := regexp.Compile(nameRegExp)
	if err != nil {
		return neutron.SecurityGroupV2{}, err
	}
	neutronClient := c.environ.neutron()
	allGroups, err := neutronClient.ListSecurityGroupsV2()
	if err != nil {
		handleCredentialError(err, ctx)
		return neutron.SecurityGroupV2{}, err
	}
	var matchingGroups []neutron.SecurityGroupV2
	for _, group := range allGroups {
		if re.MatchString(group.Name) {
			matchingGroups = append(matchingGroups, group)
		}
	}
	numMatching := len(matchingGroups)
	if numMatching == 0 {
		return neutron.SecurityGroupV2{}, errors.NotFoundf("security groups matching %q", nameRegExp)
	} else if numMatching > 1 {
		return neutron.SecurityGroupV2{}, errors.New(fmt.Sprintf("%d security groups found matching %q, expected 1", numMatching, nameRegExp))
	}
	return matchingGroups[0], nil
}

func (c *neutronFirewaller) openPortsInGroup(ctx context.ProviderCallContext, nameRegExp string, rules firewall.IngressRules) error {
	group, err := c.matchingGroup(ctx, nameRegExp)
	if err != nil {
		return errors.Trace(err)
	}
	neutronClient := c.environ.neutron()
	ruleInfo := rulesToRuleInfo(group.Id, rules)
	for _, rule := range ruleInfo {
		_, err := neutronClient.CreateSecurityGroupRuleV2(rule)
		if err != nil {
			handleCredentialError(err, ctx)
			// TODO: if err is not rule already exists, raise?
			logger.Debugf("error creating security group rule: %v", err.Error())
		}
	}
	return nil
}

// secGroupMatchesIngressRule checks if supplied nova security group rule matches the ingress rule
func secGroupMatchesIngressRule(secGroupRule neutron.SecurityGroupRuleV2, rule firewall.IngressRule) bool {
	if secGroupRule.IPProtocol == nil ||
		secGroupRule.PortRangeMax == nil || *secGroupRule.PortRangeMax == 0 ||
		secGroupRule.PortRangeMin == nil || *secGroupRule.PortRangeMin == 0 {
		return false
	}
	portsMatch := *secGroupRule.IPProtocol == rule.PortRange.Protocol &&
		*secGroupRule.PortRangeMin == rule.PortRange.FromPort &&
		*secGroupRule.PortRangeMax == rule.PortRange.ToPort
	if !portsMatch {
		return false
	}
	// The ports match, so if the security group RemoteIPPrefix matches *any* of the
	// rule's source ranges, then that's a match.
	if len(rule.SourceCIDRs) == 0 {
		return secGroupRule.RemoteIPPrefix == "" || secGroupRule.RemoteIPPrefix == "0.0.0.0/0"
	}
	return rule.SourceCIDRs.Contains(secGroupRule.RemoteIPPrefix)
}

func (c *neutronFirewaller) closePortsInGroup(ctx context.ProviderCallContext, nameRegExp string, rules firewall.IngressRules) error {
	if len(rules) == 0 {
		return nil
	}
	group, err := c.matchingGroup(ctx, nameRegExp)
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
			if err := neutronClient.DeleteSecurityGroupRuleV2(p.Id); err != nil {
				if gooseerrors.IsNotFound(err) {
					break
				}
				handleCredentialError(err, ctx)
				return errors.Trace(err)
			}

			// The rule to be removed may contain multiple CIDRs;
			// even though we matched it to one of the group rules
			// we should keep searching other rules whose IPPrefix
			// may match one of the other CIDRs.
		}
	}
	return nil
}

func (c *neutronFirewaller) ingressRulesInGroup(ctx context.ProviderCallContext, nameRegexp string) (rules firewall.IngressRules, err error) {
	group, err := c.matchingGroup(ctx, nameRegexp)
	if err != nil {
		return nil, errors.Trace(err)
	}
	// Keep track of all the RemoteIPPrefixes for each port range.
	portSourceCIDRs := make(map[corenetwork.PortRange]*[]string)
	for _, p := range group.Rules {
		// Skip the default Security Group Rules created by Neutron
		if p.Direction == "egress" {
			continue
		}
		portRange := corenetwork.PortRange{
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
		rules = append(rules, firewall.NewIngressRule(portRange, *sourceCIDRs...))
	}
	if err := rules.Validate(); err != nil {
		return nil, errors.Trace(err)
	}
	rules.Sort()
	return rules, nil
}

func replaceControllerUUID(oldName, controllerUUID string) (string, error) {
	if !extractControllerRe.MatchString(oldName) {
		return "", errors.Errorf("unexpected security group name format for %q", oldName)
	}
	newName := extractControllerRe.ReplaceAllString(
		oldName,
		"${prefix}"+controllerUUID+"${suffix}",
	)
	return newName, nil
}
