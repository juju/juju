// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package azure

import (
	"context"
	"fmt"
	"strings"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/network/armnetwork"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/resources/armresources"
	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"github.com/juju/names/v6"

	"github.com/juju/juju/core/instance"
	corenetwork "github.com/juju/juju/core/network"
	"github.com/juju/juju/core/network/firewall"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/envcontext"
	"github.com/juju/juju/internal/provider/azure/internal/errorutils"
)

type azureInstance struct {
	vmName            string
	provisioningState armresources.ProvisioningState
	provisioningError string
	env               *azureEnviron
	networkInterfaces []*armnetwork.Interface
	publicIPAddresses []*armnetwork.PublicIPAddress
}

// Id is specified in the Instance interface.
func (inst *azureInstance) Id() instance.Id {
	// Note: we use Name and not Id, since all VM operations are in
	// terms of the VM name (qualified by resource group). The ID is
	// an internal detail.
	return instance.Id(inst.vmName)
}

// Status is specified in the Instance interface.
func (inst *azureInstance) Status(ctx envcontext.ProviderCallContext) instance.Status {
	var instanceStatus status.Status
	message := string(inst.provisioningState)
	switch inst.provisioningState {
	case armresources.ProvisioningStateSucceeded:
		// TODO(axw) once a VM has been started, we should
		// start using its power state to show if it's
		// really running or not. This is just a nice to
		// have, since we should not expect a VM to ever
		// be stopped.
		instanceStatus = status.Running
		message = ""
	case armresources.ProvisioningStateDeleting, armresources.ProvisioningStateFailed:
		instanceStatus = status.ProvisioningError
		message = inst.provisioningError
	case armresources.ProvisioningStateCreating:
		message = ""
		fallthrough
	default:
		instanceStatus = status.Provisioning
	}
	return instance.Status{
		Status:  instanceStatus,
		Message: message,
	}
}

// setInstanceAddresses queries Azure for the NICs and public IPs associated
// with the given set of instances. This assumes that the instances'
// VirtualMachines are up-to-date, and that there are no concurrent accesses
// to the instances.
func (env *azureEnviron) setInstanceAddresses(
	ctx context.Context,
	resourceGroup string,
	instances []*azureInstance,
) (err error) {
	instanceNics, err := env.instanceNetworkInterfaces(ctx, resourceGroup)
	if err != nil {
		return errors.Annotate(err, "listing network interfaces")
	}
	instancePips, err := env.instancePublicIPAddresses(ctx, resourceGroup)
	if err != nil {
		return errors.Annotate(err, "listing public IP addresses")
	}
	for _, inst := range instances {
		inst.networkInterfaces = instanceNics[inst.Id()]
		inst.publicIPAddresses = instancePips[inst.Id()]
	}
	return nil
}

// instanceNetworkInterfaces lists all network interfaces in the resource
// group, and returns a mapping from instance ID to the network interfaces
// associated with that instance.
func (env *azureEnviron) instanceNetworkInterfaces(
	ctx context.Context,
	resourceGroup string,
) (map[instance.Id][]*armnetwork.Interface, error) {
	nicClient, err := env.interfacesClient()
	if err != nil {
		return nil, errors.Trace(err)
	}
	pager := nicClient.NewListPager(resourceGroup, nil)
	instanceNics := make(map[instance.Id][]*armnetwork.Interface)
	for pager.More() {
		next, err := pager.NextPage(ctx)
		if err != nil {
			return nil, env.HandleCredentialError(ctx, errors.Annotate(err, "listing network interfaces"))
		}
		for _, nic := range next.Value {
			instanceId := instance.Id(toValue(nic.Tags[jujuMachineNameTag]))
			instanceNics[instanceId] = append(instanceNics[instanceId], nic)
		}
	}
	return instanceNics, nil
}

// interfacePublicIPAddresses lists all public IP addresses in the resource
// group, and returns a mapping from instance ID to the public IP addresses
// associated with that instance.
func (env *azureEnviron) instancePublicIPAddresses(
	ctx context.Context,
	resourceGroup string,
) (map[instance.Id][]*armnetwork.PublicIPAddress, error) {
	pipClient, err := env.publicAddressesClient()
	if err != nil {
		return nil, errors.Trace(err)
	}
	pager := pipClient.NewListPager(resourceGroup, nil)
	instancePips := make(map[instance.Id][]*armnetwork.PublicIPAddress)
	for pager.More() {
		next, err := pager.NextPage(ctx)
		if err != nil {
			return nil, env.HandleCredentialError(ctx, errors.Annotate(err, "listing public IP addresses"))
		}
		for _, pip := range next.Value {
			instanceId := instance.Id(toValue(pip.Tags[jujuMachineNameTag]))
			instancePips[instanceId] = append(instancePips[instanceId], pip)
		}
	}
	return instancePips, nil
}

// Addresses is specified in the Instance interface.
func (inst *azureInstance) Addresses(ctx envcontext.ProviderCallContext) (corenetwork.ProviderAddresses, error) {
	addresses := make([]corenetwork.ProviderAddress, 0, len(inst.networkInterfaces)+len(inst.publicIPAddresses))
	for _, nic := range inst.networkInterfaces {
		if nic.Properties == nil {
			continue
		}
		for _, ipConfiguration := range nic.Properties.IPConfigurations {
			if ipConfiguration.Properties == nil || ipConfiguration.Properties.PrivateIPAddress == nil {
				continue
			}
			privateIpAddress := ipConfiguration.Properties.PrivateIPAddress
			addresses = append(addresses, corenetwork.NewMachineAddress(
				toValue(privateIpAddress),
				corenetwork.WithScope(corenetwork.ScopeCloudLocal),
			).AsProviderAddress())
		}
	}
	for _, pip := range inst.publicIPAddresses {
		if pip.Properties == nil || pip.Properties.IPAddress == nil {
			continue
		}
		addresses = append(addresses, corenetwork.NewMachineAddress(
			toValue(pip.Properties.IPAddress),
			corenetwork.WithScope(corenetwork.ScopePublic),
		).AsProviderAddress())
	}
	return addresses, nil
}

type securityGroupInfo struct {
	resourceGroup  string
	securityGroup  *armnetwork.SecurityGroup
	primaryAddress corenetwork.SpaceAddress
}

// primarySecurityGroupInfo returns info for the NIC's primary corenetwork.Address
// for the internal virtual network, and any security group on the subnet.
// The address is used to identify the machine in network security rules.
func primarySecurityGroupInfo(ctx context.Context, env *azureEnviron, nic *armnetwork.Interface) (*securityGroupInfo, error) {
	if nic == nil || nic.Properties == nil {
		return nil, errors.NotFoundf("internal network address or security group")
	}
	subnets, err := env.subnetsClient()
	if err != nil {
		return nil, errors.Trace(err)
	}
	for _, ipConfiguration := range nic.Properties.IPConfigurations {
		if ipConfiguration.Properties == nil {
			continue
		}
		if !toValue(ipConfiguration.Properties.Primary) {
			continue
		}
		privateIpAddress := ipConfiguration.Properties.PrivateIPAddress
		if privateIpAddress == nil {
			continue
		}
		securityGroup := nic.Properties.NetworkSecurityGroup
		if securityGroup == nil && ipConfiguration.Properties.Subnet != nil {
			idParts := strings.Split(toValue(ipConfiguration.Properties.Subnet.ID), "/")
			lenParts := len(idParts)
			subnet, err := subnets.Get(ctx, idParts[lenParts-7], idParts[lenParts-3], idParts[lenParts-1], &armnetwork.SubnetsClientGetOptions{
				Expand: to.Ptr("networkSecurityGroup"),
			})
			if err != nil {
				return nil, errors.Trace(err)
			}
			if subnet.Properties != nil {
				securityGroup = subnet.Properties.NetworkSecurityGroup
			}
		}
		if securityGroup == nil {
			continue
		}

		idParts := strings.Split(toValue(securityGroup.ID), "/")
		resourceGroup := idParts[len(idParts)-5]
		return &securityGroupInfo{
			resourceGroup: resourceGroup,
			securityGroup: securityGroup,
			primaryAddress: corenetwork.NewSpaceAddress(
				toValue(privateIpAddress),
				corenetwork.WithScope(corenetwork.ScopeCloudLocal),
			),
		}, nil
	}
	return nil, errors.NotFoundf("internal network address or security group")
}

// getSecurityGroupInfo gets the security group information for
// each NIC on the instance.
func (inst *azureInstance) getSecurityGroupInfo(ctx context.Context) ([]securityGroupInfo, error) {
	return getSecurityGroupInfoForInterfaces(ctx, inst.env, inst.networkInterfaces)
}

func getSecurityGroupInfoForInterfaces(ctx context.Context, env *azureEnviron, networkInterfaces []*armnetwork.Interface) ([]securityGroupInfo, error) {
	groupsByName := make(map[string]securityGroupInfo)
	for _, nic := range networkInterfaces {
		info, err := primarySecurityGroupInfo(ctx, env, nic)
		if errors.Is(err, errors.NotFound) {
			continue
		}
		if err != nil {
			return nil, errors.Trace(err)
		}
		name := toValue(info.securityGroup.Name)
		if _, ok := groupsByName[name]; ok {
			continue
		}
		groupsByName[name] = *info
	}
	var result []securityGroupInfo
	for _, sg := range groupsByName {
		result = append(result, sg)
	}
	return result, nil
}

// OpenPorts is specified in the Instance interface.
func (inst *azureInstance) OpenPorts(ctx envcontext.ProviderCallContext, machineId string, rules firewall.IngressRules) error {
	securityGroupInfos, err := inst.getSecurityGroupInfo(ctx)
	if err != nil {
		return errors.Trace(err)
	}
	for _, info := range securityGroupInfos {
		if err := inst.openPortsOnGroup(ctx, machineId, info, rules); err != nil {
			return errors.Annotatef(err,
				"opening ports on security group %q on machine %q", toValue(info.securityGroup.Name), machineId)
		}
	}
	return nil
}

func (inst *azureInstance) openPortsOnGroup(
	ctx envcontext.ProviderCallContext,
	machineId string, nsgInfo securityGroupInfo, rules firewall.IngressRules,
) error {
	nsg := nsgInfo.securityGroup
	if nsg.Properties == nil {
		nsg.Properties = &armnetwork.SecurityGroupPropertiesFormat{}
	}

	// Create rules one at a time; this is necessary to avoid trampling
	// on changes made by the provisioner. We still record rules in the
	// NSG in memory, so we can easily tell which priorities are available.
	vmName := resourceName(names.NewMachineTag(machineId))
	prefix := instanceNetworkSecurityRulePrefix(instance.Id(vmName))

	securityRules, err := inst.env.securityRulesClient()
	if err != nil {
		return errors.Trace(err)
	}
	singleSourceIngressRules := explodeIngressRules(rules)
	for _, rule := range singleSourceIngressRules {
		ruleName := securityRuleName(prefix, rule)

		// Check if the rule already exists; OpenPorts must be idempotent.
		var found bool
		for _, rule := range nsg.Properties.SecurityRules {
			if toValue(rule.Name) == ruleName {
				found = true
				break
			}
		}
		if found {
			logger.Debugf(ctx, "security rule %q already exists", ruleName)
			continue
		}
		logger.Debugf(ctx, "creating security rule %q", ruleName)

		priority, err := nextSecurityRulePriority(nsg, securityRuleInternalMax+1, securityRuleMax)
		if err != nil {
			return errors.Annotatef(err, "getting security rule priority for %q", rule)
		}

		var protocol armnetwork.SecurityRuleProtocol
		switch rule.PortRange.Protocol {
		case "tcp":
			protocol = armnetwork.SecurityRuleProtocolTCP
		case "udp":
			protocol = armnetwork.SecurityRuleProtocolUDP
		default:
			return errors.Errorf("invalid protocol %q", rule.PortRange.Protocol)
		}

		var portRange string
		if rule.PortRange.FromPort != rule.PortRange.ToPort {
			portRange = fmt.Sprintf("%d-%d", rule.PortRange.FromPort, rule.PortRange.ToPort)
		} else {
			portRange = fmt.Sprint(rule.PortRange.FromPort)
		}

		// rule has a single source CIDR
		from := rule.SourceCIDRs.SortedValues()[0]
		securityRule := armnetwork.SecurityRule{
			Properties: &armnetwork.SecurityRulePropertiesFormat{
				Description:              to.Ptr(rule.String()),
				Protocol:                 to.Ptr(protocol),
				SourcePortRange:          to.Ptr("*"),
				DestinationPortRange:     to.Ptr(portRange),
				SourceAddressPrefix:      to.Ptr(from),
				DestinationAddressPrefix: to.Ptr(nsgInfo.primaryAddress.Value),
				Access:                   to.Ptr(armnetwork.SecurityRuleAccessAllow),
				Priority:                 to.Ptr(priority),
				Direction:                to.Ptr(armnetwork.SecurityRuleDirectionInbound),
			},
		}
		poller, err := securityRules.BeginCreateOrUpdate(
			ctx,
			nsgInfo.resourceGroup, toValue(nsg.Name), ruleName, securityRule,
			nil,
		)
		if err == nil {
			_, err = poller.PollUntilDone(ctx, nil)
		}
		if err != nil {
			return inst.env.HandleCredentialError(ctx, errors.Annotatef(err, "creating security rule for %q", ruleName))
		}
		nsg.Properties.SecurityRules = append(nsg.Properties.SecurityRules, to.Ptr(securityRule))
	}
	return nil
}

// ClosePorts is specified in the Instance interface.
func (inst *azureInstance) ClosePorts(ctx envcontext.ProviderCallContext, machineId string, rules firewall.IngressRules) error {
	securityGroupInfos, err := inst.getSecurityGroupInfo(ctx)
	if err != nil {
		return errors.Trace(err)
	}
	for _, info := range securityGroupInfos {
		if err := inst.closePortsOnGroup(ctx, machineId, info, rules); err != nil {
			return errors.Annotatef(err,
				"closing ports on security group %q on machine %q", toValue(info.securityGroup.Name), machineId)
		}
	}
	return nil
}

func (inst *azureInstance) closePortsOnGroup(
	ctx envcontext.ProviderCallContext,
	machineId string, nsgInfo securityGroupInfo, rules firewall.IngressRules,
) error {
	// Delete rules one at a time; this is necessary to avoid trampling
	// on changes made by the provisioner.
	vmName := resourceName(names.NewMachineTag(machineId))
	prefix := instanceNetworkSecurityRulePrefix(instance.Id(vmName))

	securityRules, err := inst.env.securityRulesClient()
	if err != nil {
		return errors.Trace(err)
	}
	singleSourceIngressRules := explodeIngressRules(rules)
	for _, rule := range singleSourceIngressRules {
		ruleName := securityRuleName(prefix, rule)
		logger.Debugf(ctx, "deleting security rule %q", ruleName)
		poller, err := securityRules.BeginDelete(
			ctx,
			nsgInfo.resourceGroup, toValue(nsgInfo.securityGroup.Name), ruleName,
			nil,
		)
		if err == nil {
			_, err = poller.PollUntilDone(ctx, nil)
		}
		if err != nil && !errorutils.IsNotFoundError(err) {
			return inst.env.HandleCredentialError(ctx, errors.Annotatef(err, "deleting security rule %q", ruleName))
		}
	}
	return nil
}

// IngressRules is specified in the Instance interface.
func (inst *azureInstance) IngressRules(ctx envcontext.ProviderCallContext, machineId string) (firewall.IngressRules, error) {
	// The rules to use will be those on the primary network interface.
	var info *securityGroupInfo
	for _, nic := range inst.networkInterfaces {
		if nic.Properties == nil || !toValue(nic.Properties.Primary) {
			continue
		}
		var err error
		info, err = primarySecurityGroupInfo(ctx, inst.env, nic)
		if errors.Is(err, errors.NotFound) {
			continue
		}
		if err != nil {
			return nil, errors.Trace(err)
		}
		break
	}
	if info == nil {
		return nil, nil
	}
	rules, err := inst.ingressRulesForGroup(ctx, machineId, info)
	if err != nil {
		return rules, errors.Trace(err)
	}
	rules.Sort()
	return rules, nil
}

func (inst *azureInstance) ingressRulesForGroup(ctx envcontext.ProviderCallContext, machineId string, nsgInfo *securityGroupInfo) (rules firewall.IngressRules, err error) {
	securityGroups, err := inst.env.securityGroupsClient()
	if err != nil {
		return nil, errors.Trace(err)
	}
	nsg, err := securityGroups.Get(ctx, nsgInfo.resourceGroup, toValue(nsgInfo.securityGroup.Name), nil)
	if err != nil {
		return nil, inst.env.HandleCredentialError(ctx, errors.Annotatef(err, "querying network security group"))
	}
	if nsg.Properties == nil || len(nsg.Properties.SecurityRules) == 0 {
		return nil, nil
	}

	vmName := resourceName(names.NewMachineTag(machineId))
	prefix := instanceNetworkSecurityRulePrefix(instance.Id(vmName))

	// Keep track of all the SourceAddressPrefixes for each port range.
	portSourceCIDRs := make(map[corenetwork.PortRange]*[]string)
	for _, rule := range nsg.Properties.SecurityRules {
		if rule.Properties == nil {
			continue
		}
		if toValue(rule.Properties.Direction) != armnetwork.SecurityRuleDirectionInbound {
			continue
		}
		if toValue(rule.Properties.Access) != armnetwork.SecurityRuleAccessAllow {
			continue
		}
		if toValue(rule.Properties.Priority) <= securityRuleInternalMax {
			continue
		}
		if !strings.HasPrefix(toValue(rule.Name), prefix) {
			continue
		}

		var portRange corenetwork.PortRange
		if toValue(rule.Properties.DestinationPortRange) == "*" {
			portRange.FromPort = 1
			portRange.ToPort = 65535
		} else {
			portRange, err = corenetwork.ParsePortRange(
				toValue(rule.Properties.DestinationPortRange),
			)
			if err != nil {
				return nil, errors.Annotatef(
					err, "parsing port range for security rule %q",
					toValue(rule.Name),
				)
			}
		}

		var protocols []string
		switch toValue(rule.Properties.Protocol) {
		case armnetwork.SecurityRuleProtocolTCP:
			protocols = []string{"tcp"}
		case armnetwork.SecurityRuleProtocolUDP:
			protocols = []string{"udp"}
		default:
			protocols = []string{"tcp", "udp"}
		}

		// Record the SourceAddressPrefix for the port range.
		remotePrefix := toValue(rule.Properties.SourceAddressPrefix)
		if remotePrefix == "" || remotePrefix == "*" {
			remotePrefix = "0.0.0.0/0"
		}
		for _, protocol := range protocols {
			portRange.Protocol = protocol
			sourceCIDRs, ok := portSourceCIDRs[portRange]
			if !ok {
				sourceCIDRs = &[]string{}
				portSourceCIDRs[portRange] = sourceCIDRs
			}
			*sourceCIDRs = append(*sourceCIDRs, remotePrefix)
		}
	}
	// Combine all the port ranges and remote prefixes.
	for portRange, sourceCIDRs := range portSourceCIDRs {
		rules = append(rules, firewall.NewIngressRule(portRange, *sourceCIDRs...))
	}
	if err := rules.Validate(); err != nil {
		return nil, errors.Trace(err)
	}
	return rules, nil
}

// deleteInstanceNetworkSecurityRules deletes network security rules in the
// internal network security group that correspond to the specified machine.
//
// This is expected to delete *all* security rules related to the instance,
// i.e. both the ones opened by OpenPorts above, and the ones opened for API
// access.
func deleteInstanceNetworkSecurityRules(
	ctx envcontext.ProviderCallContext,
	env *azureEnviron, id instance.Id,
	networkInterfaces []*armnetwork.Interface,
) error {
	securityGroupInfos, err := getSecurityGroupInfoForInterfaces(ctx, env, networkInterfaces)
	if err != nil {
		return errors.Trace(err)
	}
	securityRules, err := env.securityRulesClient()
	if err != nil {
		return errors.Trace(err)
	}

	for _, info := range securityGroupInfos {
		if err := deleteSecurityRules(
			ctx, env.CredentialInvalidator,
			id, info,
			securityRules,
		); err != nil {
			return errors.Trace(err)
		}
	}
	return nil
}

func deleteSecurityRules(
	ctx context.Context,
	invalidator environs.CredentialInvalidator,
	id instance.Id,
	nsgInfo securityGroupInfo,
	securityRuleClient *armnetwork.SecurityRulesClient,
) error {
	nsg := nsgInfo.securityGroup
	if nsg.Properties == nil {
		return nil
	}
	prefix := instanceNetworkSecurityRulePrefix(id)
	for _, rule := range nsg.Properties.SecurityRules {
		ruleName := toValue(rule.Name)
		if !strings.HasPrefix(ruleName, prefix) {
			continue
		}
		poller, err := securityRuleClient.BeginDelete(
			ctx,
			nsgInfo.resourceGroup,
			*nsg.Name,
			ruleName,
			nil,
		)
		if err != nil {
			return errors.Annotatef(err, "deleting security rule %q", ruleName)
		}
		_, err = poller.PollUntilDone(ctx, nil)
		if err != nil && !errorutils.IsNotFoundError(err) {
			_, invalidationErr := errorutils.HandleCredentialError(ctx, invalidator, errors.Annotatef(err, "deleting security rule %q", ruleName))
			return invalidationErr
		}
	}
	return nil
}

// instanceNetworkSecurityRulePrefix returns the unique prefix for network
// security rule names that relate to the instance with the given ID.
func instanceNetworkSecurityRulePrefix(id instance.Id) string {
	return string(id) + "-"
}

// securityRuleName returns the security rule name for the given ingress rule,
// and prefix returned by instanceNetworkSecurityRulePrefix.
func securityRuleName(prefix string, rule firewall.IngressRule) string {
	ruleName := fmt.Sprintf("%s%s-%d", prefix, rule.PortRange.Protocol, rule.PortRange.FromPort)
	if rule.PortRange.FromPort != rule.PortRange.ToPort {
		ruleName += fmt.Sprintf("-%d", rule.PortRange.ToPort)
	}
	// The rule parameter must have a single source cidr.
	// Ensure the rule name can be a valid URL path component.
	var cidr string
	if rule.SourceCIDRs.IsEmpty() {
		cidr = firewall.AllNetworksIPV4CIDR
	} else {
		cidr = rule.SourceCIDRs.SortedValues()[0]
	}
	if cidr != firewall.AllNetworksIPV4CIDR && cidr != "*" {
		cidr = strings.Replace(cidr, ".", "-", -1)
		cidr = strings.Replace(cidr, "::", "-", -1)
		cidr = strings.Replace(cidr, "/", "-", -1)
		ruleName = fmt.Sprintf("%s-cidr-%s", ruleName, cidr)
	}
	return ruleName
}

// explodeIngressRules creates a slice of ingress rules, each rule in the
// result having a single source CIDR. The results contain a copy of each
// specified rule with each copy having one of the source CIDR values,
func explodeIngressRules(inRules firewall.IngressRules) firewall.IngressRules {
	// If any rule has an empty source CIDR slice, a default
	// source value of "*" is used.
	var singleSourceIngressRules firewall.IngressRules
	for _, rule := range inRules {
		sourceCIDRs := rule.SourceCIDRs
		if len(sourceCIDRs) == 0 {
			sourceCIDRs = set.NewStrings("*")
		}
		for _, sr := range sourceCIDRs.SortedValues() {
			singleSourceIngressRules = append(singleSourceIngressRules, firewall.NewIngressRule(rule.PortRange, sr))
		}
	}
	return singleSourceIngressRules
}
