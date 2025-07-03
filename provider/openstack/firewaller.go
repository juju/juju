// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package openstack

import (
	"fmt"
	"regexp"
	"strings"
	"sync"
	"time"

	gooseerrors "github.com/go-goose/goose/v5/errors"
	"github.com/go-goose/goose/v5/neutron"
	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/retry"

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

var shortRetryStrategy = retry.CallArgs{
	Clock:       clock.WallClock,
	MaxDuration: 5 * time.Second,
	Delay:       time.Second,
	BackoffFunc: retry.ExpBackoff(time.Second, 5*time.Second, 1.5, true),
}

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

	// OpenModelPorts opens the given port ranges on the model firewall
	OpenModelPorts(ctx context.ProviderCallContext, rules firewall.IngressRules) error

	// CloseModelPorts Closes the given port ranges on the model firewall
	CloseModelPorts(ctx context.ProviderCallContext, rules firewall.IngressRules) error

	// ModelIngressRules returns the set of ingress rules on the model firewall.
	// The rules are returned as sorted by network.SortIngressRules().
	// It is expected that there be only one ingress rule result for a given
	// port range - the rule's SourceCIDRs will contain all applicable source
	// address rules for that port range.
	// If the model security group doesn't exist, return a NotFound error
	ModelIngressRules(ctx context.ProviderCallContext) (firewall.IngressRules, error)

	// DeleteMachineGroup delete's the security group specific to the provided machine.
	// When in 'instance' firewall mode, each instance in a model is assigned its own
	// security group, with a lifecycle matching that of the instance itself.
	// In 'global' mode, all security groups are model scoped, and have lifecycles
	// matching the model, so this method will remove no groups.
	DeleteMachineGroup(ctx context.ProviderCallContext, machineId string) error

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
	SetUpGroups(ctx context.ProviderCallContext, controllerUUID, machineID string) ([]string, error)

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

func (c *neutronFirewaller) deleteSecurityGroupsMatchingName(
	ctx context.ProviderCallContext,
	deleteSecurityGroups func(ctx context.ProviderCallContext, groups []neutron.SecurityGroupV2, match func(name string) bool) error,
	match func(name string) bool,
) error {
	neutronClient := c.environ.neutron()
	securityGroups, err := neutronClient.ListSecurityGroupsV2()
	if err != nil {
		handleCredentialError(err, ctx)
		return errors.Annotate(err, "cannot list security groups")
	}
	return deleteSecurityGroups(ctx, securityGroups, match)
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

func (c *firewallerBase) jujuGroupPrefixRegexp() string {
	cfg := c.environ.Config()
	return fmt.Sprintf("juju-.*-%v", cfg.UUID())
}

func (c *firewallerBase) machineGroupRegexp(machineID string) string {
	// we are only looking to match 1 machine
	return fmt.Sprintf("%s-%s$", c.jujuGroupPrefixRegexp(), machineID)
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
func (c *neutronFirewaller) SetUpGroups(ctx context.ProviderCallContext, controllerUUID, machineID string) ([]string, error) {
	jujuGroup, err := c.ensureGroup(c.jujuGroupName(controllerUUID), true)
	if err != nil {
		return nil, errors.Trace(err)
	}
	var machineGroup neutron.SecurityGroupV2
	switch c.environ.Config().FirewallMode() {
	case config.FwInstance:
		machineGroup, err = c.ensureGroup(c.machineGroupName(controllerUUID, machineID), false)
	case config.FwGlobal:
		machineGroup, err = c.ensureGroup(c.globalGroupName(controllerUUID), false)
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

// zeroGroup holds the zero security group.
var zeroGroup neutron.SecurityGroupV2

// ensureGroup returns the security group with name and rules.
// If a group with name does not exist, one will be created.
// If it exists, its permissions are set to rules.
func (c *neutronFirewaller) ensureGroup(name string, isModelGroup bool) (neutron.SecurityGroupV2, error) {
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

	if !isModelGroup {
		return group, nil
	}

	if err := c.ensureInternalRules(neutronClient, group); err != nil {
		return zeroGroup, errors.Annotate(err, "failed to enable internal model rules")
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

func (c *neutronFirewaller) ensureInternalRules(neutronClient *neutron.Client, group neutron.SecurityGroupV2) error {
	rules := []neutron.RuleInfoV2{
		{
			Direction:     "ingress",
			IPProtocol:    "tcp",
			PortRangeMin:  1,
			PortRangeMax:  65535,
			EthernetType:  "IPv6",
			ParentGroupId: group.Id,
			RemoteGroupId: group.Id,
		},
		{
			Direction:     "ingress",
			IPProtocol:    "tcp",
			PortRangeMin:  1,
			PortRangeMax:  65535,
			ParentGroupId: group.Id,
			RemoteGroupId: group.Id,
		},
		{
			Direction:     "ingress",
			IPProtocol:    "udp",
			PortRangeMin:  1,
			PortRangeMax:  65535,
			EthernetType:  "IPv6",
			ParentGroupId: group.Id,
			RemoteGroupId: group.Id,
		},
		{
			Direction:     "ingress",
			IPProtocol:    "udp",
			PortRangeMin:  1,
			PortRangeMax:  65535,
			ParentGroupId: group.Id,
			RemoteGroupId: group.Id,
		},
		{
			Direction:     "ingress",
			IPProtocol:    "icmp",
			EthernetType:  "IPv6",
			ParentGroupId: group.Id,
			RemoteGroupId: group.Id,
		},
		{
			Direction:     "ingress",
			IPProtocol:    "icmp",
			ParentGroupId: group.Id,
			RemoteGroupId: group.Id,
		},
	}
	for _, rule := range rules {
		if _, err := neutronClient.CreateSecurityGroupRuleV2(rule); err != nil && !gooseerrors.IsDuplicateValue(err) {
			return err
		}
	}
	return nil
}

func (c *neutronFirewaller) deleteSecurityGroups(ctx context.ProviderCallContext, securityGroups []neutron.SecurityGroupV2) error {
	neutronClient := c.environ.neutron()
	for _, group := range securityGroups {
		deleteSecurityGroup(
			ctx,
			neutronClient.DeleteSecurityGroupV2,
			group.Name,
			group.Id,
			clock.WallClock,
		)
	}
	return nil
}

// DeleteGroups implements Firewaller interface.
func (c *neutronFirewaller) DeleteGroups(ctx context.ProviderCallContext, names ...string) error {
	var groupsToDelete []neutron.SecurityGroupV2
	for _, name := range names {
		group, err := c.getSecurityGroupByName(ctx, name)
		if err != nil && !errors.Is(err, errors.NotFound) {
			return err
		}
		groupsToDelete = append(groupsToDelete, group)
	}

	return c.deleteSecurityGroups(ctx, groupsToDelete)
}

func (c *neutronFirewaller) listAndFilterSecurityGroups(ctx context.ProviderCallContext, re *regexp.Regexp) ([]neutron.SecurityGroupV2, error) {
	neutronClient := c.environ.neutron()
	securityGroups, err := neutronClient.ListSecurityGroupsV2()
	var securityGroupsToDelete []neutron.SecurityGroupV2
	if err != nil {
		handleCredentialError(err, ctx)
		return securityGroupsToDelete, errors.Annotate(err, "cannot list security groups")
	}

	for _, group := range securityGroups {
		if re.MatchString(group.Name) {
			securityGroupsToDelete = append(securityGroupsToDelete, group)
		}
	}

	return securityGroupsToDelete, nil
}

// DeleteAllControllerGroups implements Firewaller interface.
func (c *neutronFirewaller) DeleteAllControllerGroups(ctx context.ProviderCallContext, controllerUUID string) error {
	controllerGroupPrefix := c.jujuControllerGroupPrefix(controllerUUID)
	re, err := regexp.Compile("^" + controllerGroupPrefix)
	if err != nil {
		return errors.Trace(err)
	}

	securityGroups, err := c.listAndFilterSecurityGroups(ctx, re)
	if err != nil {
		return errors.Trace(err)
	}

	return c.deleteSecurityGroups(ctx, securityGroups)
}

// DeleteAllModelGroups implements Firewaller interface.
func (c *neutronFirewaller) DeleteAllModelGroups(ctx context.ProviderCallContext) error {
	jujuGroupPrefixRegex := c.jujuGroupPrefixRegexp()
	re, err := regexp.Compile("^" + jujuGroupPrefixRegex)
	if err != nil {
		return errors.Trace(err)
	}

	securityGroups, err := c.listAndFilterSecurityGroups(ctx, re)
	if err != nil {
		return errors.Trace(err)
	}

	return c.deleteSecurityGroups(ctx, securityGroups)
}

func (c *neutronFirewaller) DeleteMachineGroup(ctx context.ProviderCallContext, machineID string) error {
	if c.environ.Config().FirewallMode() != config.FwInstance {
		return nil
	}

	group, err := c.getSecurityGroupByName(ctx, c.machineGroupName(c.environ.controllerUUID, machineID))
	if err != nil {
		return errors.Trace(err)
	}

	return c.deleteSecurityGroups(ctx, []neutron.SecurityGroupV2{group})
}

// UpdateGroupController implements Firewaller interface.
func (c *neutronFirewaller) UpdateGroupController(ctx context.ProviderCallContext, controllerUUID string) error {
	neutronClient := c.environ.neutron()
	groups, err := neutronClient.ListSecurityGroupsV2()
	if err != nil {
		handleCredentialError(err, ctx)
		return errors.Trace(err)
	}
	re, err := regexp.Compile(c.jujuGroupPrefixRegexp())
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
	if c.environ.Config().FirewallMode() != config.FwGlobal {
		return errors.Errorf("invalid firewall mode %q for opening ports on model",
			c.environ.Config().FirewallMode())
	}
	if err := c.openPortsInGroup(ctx, c.globalGroupName(c.environ.controllerUUID), rules); err != nil {
		handleCredentialError(err, ctx)
		return errors.Trace(err)
	}
	logger.Infof("opened ports in global group: %v", rules)
	return nil
}

// ClosePorts implements Firewaller interface.
func (c *neutronFirewaller) ClosePorts(ctx context.ProviderCallContext, rules firewall.IngressRules) error {
	if c.environ.Config().FirewallMode() != config.FwGlobal {
		return errors.Errorf("invalid firewall mode %q for closing ports on model",
			c.environ.Config().FirewallMode())
	}
	if err := c.closePortsInGroup(ctx, c.globalGroupName(c.environ.controllerUUID), rules); err != nil {
		handleCredentialError(err, ctx)
		return errors.Trace(err)
	}
	logger.Infof("closed ports in global group: %v", rules)
	return nil
}

// IngressRules implements Firewaller interface.
func (c *neutronFirewaller) IngressRules(ctx context.ProviderCallContext) (firewall.IngressRules, error) {
	if c.environ.Config().FirewallMode() != config.FwGlobal {
		return nil, errors.Errorf("invalid firewall mode %q for retrieving ingress rules from model",
			c.environ.Config().FirewallMode())
	}
	rules, err := c.ingressRulesInGroup(ctx, c.globalGroupName(c.environ.controllerUUID))
	if err != nil {
		handleCredentialError(err, ctx)
		return rules, errors.Trace(err)
	}
	return rules, nil
}

// OpenModelPorts implements Firewaller interface
func (c *neutronFirewaller) OpenModelPorts(ctx context.ProviderCallContext, rules firewall.IngressRules) error {
	err := c.openPortsInGroup(ctx, c.jujuGroupName(c.environ.controllerUUID), rules)
	if errors.IsNotFound(err) && !c.environ.usingSecurityGroups {
		logger.Warningf("attempted to open %v but network port security is disabled. Already open", rules)
		return nil
	}
	if err != nil {
		handleCredentialError(err, ctx)
		return errors.Trace(err)
	}
	logger.Infof("opened ports in model group: %v", rules)
	return nil
}

// CloseModelPorts implements Firewaller interface
func (c *neutronFirewaller) CloseModelPorts(ctx context.ProviderCallContext, rules firewall.IngressRules) error {
	if err := c.closePortsInGroup(ctx, c.jujuGroupName(c.environ.controllerUUID), rules); err != nil {
		handleCredentialError(err, ctx)
		return errors.Trace(err)
	}
	logger.Infof("closed ports in global group: %v", rules)
	return nil
}

// ModelIngressRules implements Firewaller interface
func (c *neutronFirewaller) ModelIngressRules(ctx context.ProviderCallContext) (firewall.IngressRules, error) {
	rules, err := c.ingressRulesInGroup(ctx, c.jujuGroupName(c.environ.controllerUUID))
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
	if err := c.openPortsInGroup(ctx, c.machineGroupName(c.environ.controllerUUID, machineID), ports); err != nil {
		handleCredentialError(err, ctx)
		return errors.Trace(err)
	}
	logger.Infof("opened ports in security group %s-%s: %v", c.environ.Config().UUID(), machineID, ports)
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
	if err := c.closePortsInGroup(ctx, c.machineGroupName(c.environ.controllerUUID, machineID), ports); err != nil {
		handleCredentialError(err, ctx)
		return errors.Trace(err)
	}
	logger.Infof("closed ports in security group %s-%s: %v", c.environ.Config().UUID(), machineID, ports)
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
	rules, err := c.ingressRulesInGroup(ctx, c.machineGroupName(c.environ.controllerUUID, machineID))
	if err != nil {
		handleCredentialError(err, ctx)
		return rules, errors.Trace(err)
	}
	return rules, err
}

// getSecurityGroupByName talks to Openstack Neutron service to get a security group by name.
// Here, argument name can be a group name for a model, machine or global.
func (c *neutronFirewaller) getSecurityGroupByName(ctx context.ProviderCallContext, name string) (neutron.SecurityGroupV2, error) {
	neutronClient := c.environ.neutron()

	// If the security group has just been created, it might not be available
	// yet. If we get not matching groups, we will retry the request using
	// shortRetryStrategy before giving up
	var matchingGroup neutron.SecurityGroupV2

	retryStrategy := shortRetryStrategy
	retryStrategy.IsFatalError = func(err error) bool {
		return !errors.IsNotFound(err)
	}
	retryStrategy.Func = func() error {
		groups, err := neutronClient.SecurityGroupByNameV2(name)
		if err != nil {
			handleCredentialError(err, ctx)
			return err
		}

		numMatching := len(groups)
		if numMatching == 0 {
			return errors.NotFoundf("security group %q", name)
		} else if numMatching > 1 {
			return errors.New(fmt.Sprintf("%d security groups found with name %q, expected 1", numMatching, name))
		}
		matchingGroup = groups[0]
		return nil
	}
	err := retry.Call(retryStrategy)
	if retry.IsAttemptsExceeded(err) || retry.IsDurationExceeded(err) {
		err = retry.LastError(err)
	}
	return matchingGroup, err
}

func (c *neutronFirewaller) openPortsInGroup(ctx context.ProviderCallContext, name string, rules firewall.IngressRules) error {
	group, err := c.getSecurityGroupByName(ctx, name)
	if err != nil {
		return errors.Trace(err)
	}
	neutronClient := c.environ.neutron()
	ruleInfo := rulesToRuleInfo(group.Id, rules)
	for _, rule := range ruleInfo {
		_, err := neutronClient.CreateSecurityGroupRuleV2(rule)
		if err != nil && !gooseerrors.IsDuplicateValue(err) {
			handleCredentialError(err, ctx)
			// TODO: if err is not rule already exists, raise?
			return fmt.Errorf("creating security group rule %q for parent group id %q using proto %q: %w", rule.Direction, rule.ParentGroupId, rule.IPProtocol, err)
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
		return secGroupRule.RemoteIPPrefix == "" ||
			secGroupRule.RemoteIPPrefix == "0.0.0.0/0" ||
			secGroupRule.RemoteIPPrefix == "::/0"
	}
	return rule.SourceCIDRs.Contains(secGroupRule.RemoteIPPrefix)
}

func (c *neutronFirewaller) closePortsInGroup(ctx context.ProviderCallContext, name string, rules firewall.IngressRules) error {
	if len(rules) == 0 {
		return nil
	}
	group, err := c.getSecurityGroupByName(ctx, name)
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

func (c *neutronFirewaller) ingressRulesInGroup(ctx context.ProviderCallContext, name string) (rules firewall.IngressRules, err error) {
	group, err := c.getSecurityGroupByName(ctx, name)
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
		// Skip internal security group rules
		if p.RemoteGroupID != "" {
			continue
		}

		portRange := corenetwork.PortRange{
			Protocol: *p.IPProtocol,
		}
		if portRange.Protocol == "ipv6-icmp" {
			portRange.Protocol = "icmp"
		}
		// NOTE: Juju firewall rule validation expects that icmp rules have port
		// values set to -1
		if p.PortRangeMin != nil {
			portRange.FromPort = *p.PortRangeMin
		} else if portRange.Protocol == "icmp" {
			portRange.FromPort = -1
		}
		if p.PortRangeMax != nil {
			portRange.ToPort = *p.PortRangeMax
		} else if portRange.Protocol == "icmp" {
			portRange.ToPort = -1
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
