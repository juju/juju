// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package azure

import (
	stdcontext "context"
	"fmt"
	"strings"

	"github.com/Azure/azure-sdk-for-go/services/network/mgmt/2018-08-01/network"
	"github.com/Azure/go-autorest/autorest/to"
	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"github.com/juju/names/v4"

	"github.com/juju/juju/core/instance"
	corenetwork "github.com/juju/juju/core/network"
	"github.com/juju/juju/core/network/firewall"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/environs/context"
	"github.com/juju/juju/provider/azure/internal/errorutils"
)

type azureInstance struct {
	vmName            string
	provisioningState string
	env               *azureEnviron
	networkInterfaces []network.Interface
	publicIPAddresses []network.PublicIPAddress
}

// Id is specified in the Instance interface.
func (inst *azureInstance) Id() instance.Id {
	// Note: we use Name and not Id, since all VM operations are in
	// terms of the VM name (qualified by resource group). The ID is
	// an internal detail.
	return instance.Id(inst.vmName)
}

// Status is specified in the Instance interface.
func (inst *azureInstance) Status(ctx context.ProviderCallContext) instance.Status {
	var instanceStatus status.Status
	message := inst.provisioningState
	switch inst.provisioningState {
	case "Succeeded":
		// TODO(axw) once a VM has been started, we should
		// start using its power state to show if it's
		// really running or not. This is just a nice to
		// have, since we should not expect a VM to ever
		// be stopped.
		instanceStatus = status.Running
		message = ""
	case "Canceled", "Failed":
		// TODO(axw) if the provisioning state is "Failed", then we
		// should use the error message from the deployment description
		// as the Message. The error details are not currently exposed
		// in the Azure SDK. See:
		//     https://github.com/Azure/azure-sdk-for-go/issues/399
		instanceStatus = status.ProvisioningError
	case "Running":
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
func setInstanceAddresses(
	ctx context.ProviderCallContext,
	resourceGroup string,
	nicClient network.InterfacesClient,
	pipClient network.PublicIPAddressesClient,
	instances []*azureInstance,
) (err error) {
	instanceNics, err := instanceNetworkInterfaces(ctx, resourceGroup, nicClient)
	if err != nil {
		return errors.Annotate(err, "listing network interfaces")
	}
	instancePips, err := instancePublicIPAddresses(ctx, resourceGroup, pipClient)
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
func instanceNetworkInterfaces(
	ctx context.ProviderCallContext,
	resourceGroup string,
	nicClient network.InterfacesClient,
) (map[instance.Id][]network.Interface, error) {
	sdkCtx := stdcontext.Background()
	nicsResult, err := nicClient.ListComplete(sdkCtx, resourceGroup)
	if err != nil {
		return nil, errorutils.HandleCredentialError(errors.Annotate(err, "listing network interfaces"), ctx)
	}
	if nicsResult.Response().IsEmpty() {
		return nil, nil
	}
	instanceNics := make(map[instance.Id][]network.Interface)
	for ; nicsResult.NotDone(); err = nicsResult.NextWithContext(sdkCtx) {
		if err != nil {
			return nil, errors.Trace(err)
		}
		nic := nicsResult.Value()
		instanceId := instance.Id(to.String(nic.Tags[jujuMachineNameTag]))
		instanceNics[instanceId] = append(instanceNics[instanceId], nic)
	}
	return instanceNics, nil
}

// interfacePublicIPAddresses lists all public IP addresses in the resource
// group, and returns a mapping from instance ID to the public IP addresses
// associated with that instance.
func instancePublicIPAddresses(
	ctx context.ProviderCallContext,
	resourceGroup string,
	pipClient network.PublicIPAddressesClient,
) (map[instance.Id][]network.PublicIPAddress, error) {
	sdkCtx := stdcontext.Background()
	pipsResult, err := pipClient.ListComplete(sdkCtx, resourceGroup)
	if err != nil {
		return nil, errorutils.HandleCredentialError(errors.Annotate(err, "listing public IP addresses"), ctx)
	}
	if pipsResult.Response().IsEmpty() {
		return nil, nil
	}
	instancePips := make(map[instance.Id][]network.PublicIPAddress)
	for ; pipsResult.NotDone(); err = pipsResult.NextWithContext(sdkCtx) {
		if err != nil {
			return nil, errors.Trace(err)
		}
		pip := pipsResult.Value()
		instanceId := instance.Id(to.String(pip.Tags[jujuMachineNameTag]))
		instancePips[instanceId] = append(instancePips[instanceId], pip)
	}
	return instancePips, nil
}

// Addresses is specified in the Instance interface.
func (inst *azureInstance) Addresses(ctx context.ProviderCallContext) (corenetwork.ProviderAddresses, error) {
	addresses := make([]corenetwork.ProviderAddress, 0, len(inst.networkInterfaces)+len(inst.publicIPAddresses))
	for _, nic := range inst.networkInterfaces {
		if nic.IPConfigurations == nil {
			continue
		}
		for _, ipConfiguration := range *nic.IPConfigurations {
			privateIpAddress := ipConfiguration.PrivateIPAddress
			if privateIpAddress == nil {
				continue
			}
			addresses = append(addresses, corenetwork.NewProviderAddress(
				to.String(privateIpAddress),
				corenetwork.WithScope(corenetwork.ScopeCloudLocal),
			))
		}
	}
	for _, pip := range inst.publicIPAddresses {
		if pip.IPAddress == nil {
			continue
		}
		addresses = append(addresses, corenetwork.NewProviderAddress(
			to.String(pip.IPAddress),
			corenetwork.WithScope(corenetwork.ScopePublic),
		))
	}
	return addresses, nil
}

type securityGroupInfo struct {
	resourceGroup  string
	securityGroup  *network.SecurityGroup
	primaryAddress corenetwork.SpaceAddress
}

// primarySecurityGroupInfo returns info for the NIC's primary corenetwork.Address
// for the internal virtual network, and any security group on the subnet.
// The address is used to identify the machine in network security rules.
func primarySecurityGroupInfo(ctx stdcontext.Context, env *azureEnviron, nic network.Interface) (*securityGroupInfo, error) {
	if nic.IPConfigurations == nil {
		return nil, errors.NotFoundf("internal network address or security group")
	}
	for _, ipConfiguration := range *nic.IPConfigurations {
		if ipConfiguration.Subnet == nil {
			continue
		}
		if !to.Bool(ipConfiguration.Primary) {
			continue
		}
		privateIpAddress := ipConfiguration.PrivateIPAddress
		if privateIpAddress == nil {
			continue
		}
		securityGroup := nic.NetworkSecurityGroup
		if securityGroup == nil {
			idParts := strings.Split(to.String(ipConfiguration.Subnet.ID), "/")
			lenParts := len(idParts)
			subnetClient := network.SubnetsClient{BaseClient: env.network}
			subnet, err := subnetClient.Get(ctx, idParts[lenParts-7], idParts[lenParts-3], idParts[lenParts-1], "networkSecurityGroup")
			if err != nil {
				return nil, errors.Trace(err)
			}
			securityGroup = subnet.NetworkSecurityGroup
		}
		if securityGroup == nil {
			continue
		}

		idParts := strings.Split(to.String(securityGroup.ID), "/")
		resourceGroup := idParts[len(idParts)-5]
		return &securityGroupInfo{
			resourceGroup: resourceGroup,
			securityGroup: securityGroup,
			primaryAddress: corenetwork.NewSpaceAddress(
				to.String(privateIpAddress),
				corenetwork.WithScope(corenetwork.ScopeCloudLocal),
			),
		}, nil
	}
	return nil, errors.NotFoundf("internal network address or security group")
}

// getSecurityGroupInfo gets the security group information for
// each NIC on the instance.
func (inst *azureInstance) getSecurityGroupInfo(ctx stdcontext.Context) ([]securityGroupInfo, error) {
	return getSecurityGroupInfoForInterfaces(ctx, inst.env, inst.networkInterfaces)
}

func getSecurityGroupInfoForInterfaces(ctx stdcontext.Context, env *azureEnviron, networkInterfaces []network.Interface) ([]securityGroupInfo, error) {
	groupsByName := make(map[string]securityGroupInfo)
	for _, nic := range networkInterfaces {
		info, err := primarySecurityGroupInfo(ctx, env, nic)
		if errors.IsNotFound(err) {
			continue
		}
		if err != nil {
			return nil, errors.Trace(err)
		}
		name := to.String(info.securityGroup.Name)
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
func (inst *azureInstance) OpenPorts(ctx context.ProviderCallContext, machineId string, rules firewall.IngressRules) error {
	sdkCtx := stdcontext.Background()
	securityGroupInfos, err := inst.getSecurityGroupInfo(sdkCtx)
	if err != nil {
		return errors.Trace(err)
	}
	for _, info := range securityGroupInfos {
		if err := inst.openPortsOnGroup(sdkCtx, ctx, machineId, info, rules); err != nil {
			return errors.Annotatef(err,
				"opening ports on security group %q on machine %q", to.String(info.securityGroup.Name), machineId)
		}
	}
	return nil
}

func (inst *azureInstance) openPortsOnGroup(
	sdkCtx stdcontext.Context,
	ctx context.ProviderCallContext,
	machineId string, nsgInfo securityGroupInfo, rules firewall.IngressRules,
) error {
	nsg := nsgInfo.securityGroup
	if nsg.SecurityRules == nil {
		nsg.SecurityRules = new([]network.SecurityRule)
	}

	// Create rules one at a time; this is necessary to avoid trampling
	// on changes made by the provisioner. We still record rules in the
	// NSG in memory, so we can easily tell which priorities are available.
	vmName := resourceName(names.NewMachineTag(machineId))
	prefix := instanceNetworkSecurityRulePrefix(instance.Id(vmName))

	singleSourceIngressRules := explodeIngressRules(rules)
	securityRuleClient := network.SecurityRulesClient{inst.env.network}
	for _, rule := range singleSourceIngressRules {
		ruleName := securityRuleName(prefix, rule)

		// Check if the rule already exists; OpenPorts must be idempotent.
		var found bool
		for _, rule := range *nsg.SecurityRules {
			if to.String(rule.Name) == ruleName {
				found = true
				break
			}
		}
		if found {
			logger.Debugf("security rule %q already exists", ruleName)
			continue
		}
		logger.Debugf("creating security rule %q", ruleName)

		priority, err := nextSecurityRulePriority(nsg, securityRuleInternalMax+1, securityRuleMax)
		if err != nil {
			return errors.Annotatef(err, "getting security rule priority for %q", rule)
		}

		var protocol network.SecurityRuleProtocol
		switch rule.PortRange.Protocol {
		case "tcp":
			protocol = network.SecurityRuleProtocolTCP
		case "udp":
			protocol = network.SecurityRuleProtocolUDP
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
		securityRule := network.SecurityRule{
			SecurityRulePropertiesFormat: &network.SecurityRulePropertiesFormat{
				Description:              to.StringPtr(rule.String()),
				Protocol:                 protocol,
				SourcePortRange:          to.StringPtr("*"),
				DestinationPortRange:     to.StringPtr(portRange),
				SourceAddressPrefix:      to.StringPtr(from),
				DestinationAddressPrefix: to.StringPtr(nsgInfo.primaryAddress.Value),
				Access:                   network.SecurityRuleAccessAllow,
				Priority:                 to.Int32Ptr(priority),
				Direction:                network.SecurityRuleDirectionInbound,
			},
		}
		_, err = securityRuleClient.CreateOrUpdate(
			sdkCtx,
			nsgInfo.resourceGroup, to.String(nsg.Name), ruleName, securityRule,
		)
		if err != nil {
			return errorutils.HandleCredentialError(errors.Annotatef(err, "creating security rule for %q", ruleName), ctx)
		}
		*nsg.SecurityRules = append(*nsg.SecurityRules, securityRule)
	}
	return nil
}

// ClosePorts is specified in the Instance interface.
func (inst *azureInstance) ClosePorts(ctx context.ProviderCallContext, machineId string, rules firewall.IngressRules) error {
	sdkCtx := stdcontext.Background()
	securityGroupInfos, err := inst.getSecurityGroupInfo(sdkCtx)
	if err != nil {
		return errors.Trace(err)
	}
	for _, info := range securityGroupInfos {
		if err := inst.closePortsOnGroup(sdkCtx, ctx, machineId, info, rules); err != nil {
			return errors.Annotatef(err,
				"closing ports on security group %q on machine %q", to.String(info.securityGroup.Name), machineId)
		}
	}
	return nil
}

func (inst *azureInstance) closePortsOnGroup(
	sdkCtx stdcontext.Context,
	ctx context.ProviderCallContext,
	machineId string, nsgInfo securityGroupInfo, rules firewall.IngressRules,
) error {
	securityRuleClient := network.SecurityRulesClient{inst.env.network}

	// Delete rules one at a time; this is necessary to avoid trampling
	// on changes made by the provisioner.
	vmName := resourceName(names.NewMachineTag(machineId))
	prefix := instanceNetworkSecurityRulePrefix(instance.Id(vmName))

	singleSourceIngressRules := explodeIngressRules(rules)
	for _, rule := range singleSourceIngressRules {
		ruleName := securityRuleName(prefix, rule)
		logger.Debugf("deleting security rule %q", ruleName)
		future, err := securityRuleClient.Delete(
			stdcontext.Background(),
			nsgInfo.resourceGroup, to.String(nsgInfo.securityGroup.Name), ruleName,
		)
		if err != nil {
			if !isNotFoundResponse(future.Response()) {
				return errors.Annotatef(err, "deleting security rule %q", ruleName)
			}
			continue
		}
		err = future.WaitForCompletionRef(sdkCtx, securityRuleClient.Client)
		if err != nil {
			return errors.Annotatef(err, "deleting security rule %q", ruleName)
		}
		result, err := future.Result(securityRuleClient)
		if err != nil && !isNotFoundResult(result) {
			return errorutils.HandleCredentialError(errors.Annotatef(err, "deleting security rule %q", ruleName), ctx)
		}
	}
	return nil
}

// IngressRules is specified in the Instance interface.
func (inst *azureInstance) IngressRules(ctx context.ProviderCallContext, machineId string) (firewall.IngressRules, error) {
	// The rules to use will be those on the primary network interface.
	sdkCtx := stdcontext.Background()
	var info *securityGroupInfo
	for _, nic := range inst.networkInterfaces {
		if !to.Bool(nic.Primary) {
			continue
		}
		var err error
		info, err = primarySecurityGroupInfo(sdkCtx, inst.env, nic)
		if errors.IsNotFound(err) {
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

func (inst *azureInstance) ingressRulesForGroup(ctx context.ProviderCallContext, machineId string, nsgInfo *securityGroupInfo) (rules firewall.IngressRules, err error) {
	nsgClient := network.SecurityGroupsClient{inst.env.network}
	nsg, err := nsgClient.Get(stdcontext.Background(), nsgInfo.resourceGroup, to.String(nsgInfo.securityGroup.Name), "")
	if err != nil {
		return nil, errorutils.HandleCredentialError(errors.Annotate(err, "querying network security group"), ctx)
	}
	if nsg.SecurityRules == nil {
		return nil, nil
	}

	vmName := resourceName(names.NewMachineTag(machineId))
	prefix := instanceNetworkSecurityRulePrefix(instance.Id(vmName))

	// Keep track of all the SourceAddressPrefixes for each port range.
	portSourceCIDRs := make(map[corenetwork.PortRange]*[]string)
	for _, rule := range *nsg.SecurityRules {
		if rule.Direction != network.SecurityRuleDirectionInbound {
			continue
		}
		if rule.Access != network.SecurityRuleAccessAllow {
			continue
		}
		if to.Int32(rule.Priority) <= securityRuleInternalMax {
			continue
		}
		if !strings.HasPrefix(to.String(rule.Name), prefix) {
			continue
		}

		var portRange corenetwork.PortRange
		if *rule.DestinationPortRange == "*" {
			portRange.FromPort = 1
			portRange.ToPort = 65535
		} else {
			portRange, err = corenetwork.ParsePortRange(
				*rule.DestinationPortRange,
			)
			if err != nil {
				return nil, errors.Annotatef(
					err, "parsing port range for security rule %q",
					to.String(rule.Name),
				)
			}
		}

		var protocols []string
		switch rule.Protocol {
		case network.SecurityRuleProtocolTCP:
			protocols = []string{"tcp"}
		case network.SecurityRuleProtocolUDP:
			protocols = []string{"udp"}
		default:
			protocols = []string{"tcp", "udp"}
		}

		// Record the SourceAddressPrefix for the port range.
		remotePrefix := to.String(rule.SourceAddressPrefix)
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
	ctx context.ProviderCallContext,
	env *azureEnviron, id instance.Id,
	networkInterfaces []network.Interface,
) error {
	securityGroupInfos, err := getSecurityGroupInfoForInterfaces(ctx, env, networkInterfaces)
	if err != nil {
		return errors.Trace(err)
	}
	for _, info := range securityGroupInfos {
		if err := deleteSecurityRules(
			ctx, ctx, id, info,
			network.SecurityRulesClient{env.network},
		); err != nil {
			return errors.Trace(err)
		}
	}
	return nil
}

func deleteSecurityRules(
	sdkCtx stdcontext.Context,
	ctx context.ProviderCallContext,
	id instance.Id,
	nsgInfo securityGroupInfo,
	securityRuleClient network.SecurityRulesClient,
) error {
	nsg := nsgInfo.securityGroup
	if nsg.SecurityRules == nil {
		return nil
	}
	prefix := instanceNetworkSecurityRulePrefix(id)
	for _, rule := range *nsg.SecurityRules {
		ruleName := to.String(rule.Name)
		if !strings.HasPrefix(ruleName, prefix) {
			continue
		}
		future, err := securityRuleClient.Delete(
			sdkCtx,
			nsgInfo.resourceGroup,
			*nsg.Name,
			ruleName,
		)
		if err != nil {
			if !isNotFoundResponse(future.Response()) {
				return errors.Annotatef(err, "deleting security rule %q", ruleName)
			}
			continue
		}
		err = future.WaitForCompletionRef(sdkCtx, securityRuleClient.Client)
		if err != nil {
			return errors.Annotatef(err, "deleting security rule %q", ruleName)
		}
		result, err := future.Result(securityRuleClient)
		if err != nil && !isNotFoundResult(result) {
			return errorutils.HandleCredentialError(errors.Annotatef(err, "deleting security rule %q", ruleName), ctx)
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
