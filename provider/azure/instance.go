// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package azure

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/Azure/azure-sdk-for-go/arm/network"
	"github.com/Azure/go-autorest/autorest"
	"github.com/Azure/go-autorest/autorest/to"
	"github.com/juju/errors"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/environs/context"
	"github.com/juju/juju/instance"
	jujunetwork "github.com/juju/juju/network"
	"github.com/juju/juju/status"
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
func (inst *azureInstance) Status(ctx context.ProviderCallContext) instance.InstanceStatus {
	instanceStatus := status.Empty
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
	return instance.InstanceStatus{
		Status:  instanceStatus,
		Message: message,
	}
}

// setInstanceAddresses queries Azure for the NICs and public IPs associated
// with the given set of instances. This assumes that the instances'
// VirtualMachines are up-to-date, and that there are no concurrent accesses
// to the instances.
func setInstanceAddresses(
	resourceGroup string,
	nicClient network.InterfacesClient,
	pipClient network.PublicIPAddressesClient,
	instances []*azureInstance,
) (err error) {
	instanceNics, err := instanceNetworkInterfaces(resourceGroup, nicClient)
	if err != nil {
		return errors.Annotate(err, "listing network interfaces")
	}
	instancePips, err := instancePublicIPAddresses(resourceGroup, pipClient)
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
	resourceGroup string,
	nicClient network.InterfacesClient,
) (map[instance.Id][]network.Interface, error) {
	nicsResult, err := nicClient.List(resourceGroup)
	if err != nil {
		return nil, errors.Annotate(err, "listing network interfaces")
	}
	if nicsResult.Value == nil || len(*nicsResult.Value) == 0 {
		return nil, nil
	}
	instanceNics := make(map[instance.Id][]network.Interface)
	for _, nic := range *nicsResult.Value {
		instanceId := instance.Id(toTags(nic.Tags)[jujuMachineNameTag])
		instanceNics[instanceId] = append(instanceNics[instanceId], nic)
	}
	return instanceNics, nil
}

// interfacePublicIPAddresses lists all public IP addresses in the resource
// group, and returns a mapping from instance ID to the public IP addresses
// associated with that instance.
func instancePublicIPAddresses(
	resourceGroup string,
	pipClient network.PublicIPAddressesClient,
) (map[instance.Id][]network.PublicIPAddress, error) {
	pipsResult, err := pipClient.List(resourceGroup)
	if err != nil {
		return nil, errors.Annotate(err, "listing public IP addresses")
	}
	if pipsResult.Value == nil || len(*pipsResult.Value) == 0 {
		return nil, nil
	}
	instancePips := make(map[instance.Id][]network.PublicIPAddress)
	for _, pip := range *pipsResult.Value {
		instanceId := instance.Id(toTags(pip.Tags)[jujuMachineNameTag])
		instancePips[instanceId] = append(instancePips[instanceId], pip)
	}
	return instancePips, nil
}

// Addresses is specified in the Instance interface.
func (inst *azureInstance) Addresses(ctx context.ProviderCallContext) ([]jujunetwork.Address, error) {
	addresses := make([]jujunetwork.Address, 0, len(inst.networkInterfaces)+len(inst.publicIPAddresses))
	for _, nic := range inst.networkInterfaces {
		if nic.IPConfigurations == nil {
			continue
		}
		for _, ipConfiguration := range *nic.IPConfigurations {
			privateIpAddress := ipConfiguration.PrivateIPAddress
			if privateIpAddress == nil {
				continue
			}
			addresses = append(addresses, jujunetwork.NewScopedAddress(
				to.String(privateIpAddress),
				jujunetwork.ScopeCloudLocal,
			))
		}
	}
	for _, pip := range inst.publicIPAddresses {
		if pip.IPAddress == nil {
			continue
		}
		addresses = append(addresses, jujunetwork.NewScopedAddress(
			to.String(pip.IPAddress),
			jujunetwork.ScopePublic,
		))
	}
	return addresses, nil
}

// primaryNetworkAddress returns the instance's primary jujunetwork.Address for
// the internal virtual network. This address is used to identify the machine in
// network security rules.
func (inst *azureInstance) primaryNetworkAddress() (jujunetwork.Address, error) {
	for _, nic := range inst.networkInterfaces {
		if nic.IPConfigurations == nil {
			continue
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
			return jujunetwork.NewScopedAddress(
				to.String(privateIpAddress),
				jujunetwork.ScopeCloudLocal,
			), nil
		}
	}
	return jujunetwork.Address{}, errors.NotFoundf("internal network address")
}

// OpenPorts is specified in the Instance interface.
func (inst *azureInstance) OpenPorts(ctx context.ProviderCallContext, machineId string, rules []jujunetwork.IngressRule) error {
	nsgClient := network.SecurityGroupsClient{inst.env.network}
	securityRuleClient := network.SecurityRulesClient{inst.env.network}
	primaryNetworkAddress, err := inst.primaryNetworkAddress()
	if err != nil {
		return errors.Trace(err)
	}

	securityGroupName := internalSecurityGroupName
	nsg, err := nsgClient.Get(inst.env.resourceGroup, securityGroupName, "")
	if err != nil {
		return errors.Annotate(err, "querying network security group")
	}

	var securityRules []network.SecurityRule
	if nsg.SecurityRules != nil {
		securityRules = *nsg.SecurityRules
	} else {
		nsg.SecurityRules = &securityRules
	}

	// Create rules one at a time; this is necessary to avoid trampling
	// on changes made by the provisioner. We still record rules in the
	// NSG in memory, so we can easily tell which priorities are available.
	vmName := resourceName(names.NewMachineTag(machineId))
	prefix := instanceNetworkSecurityRulePrefix(instance.Id(vmName))

	singleSourceIngressRules := explodeIngressRules(rules)
	for _, rule := range singleSourceIngressRules {
		ruleName := securityRuleName(prefix, rule)

		// Check if the rule already exists; OpenPorts must be idempotent.
		var found bool
		for _, rule := range securityRules {
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
			return errors.Annotatef(err, "getting security rule priority for %s", rule)
		}

		var protocol network.SecurityRuleProtocol
		switch rule.Protocol {
		case "tcp":
			protocol = network.SecurityRuleProtocolTCP
		case "udp":
			protocol = network.SecurityRuleProtocolUDP
		default:
			return errors.Errorf("invalid protocol %q", rule.Protocol)
		}

		var portRange string
		if rule.FromPort != rule.ToPort {
			portRange = fmt.Sprintf("%d-%d", rule.FromPort, rule.ToPort)
		} else {
			portRange = fmt.Sprint(rule.FromPort)
		}

		// rule has a single source CIDR
		from := rule.SourceCIDRs[0]
		securityRule := network.SecurityRule{
			SecurityRulePropertiesFormat: &network.SecurityRulePropertiesFormat{
				Description:              to.StringPtr(rule.String()),
				Protocol:                 protocol,
				SourcePortRange:          to.StringPtr("*"),
				DestinationPortRange:     to.StringPtr(portRange),
				SourceAddressPrefix:      to.StringPtr(from),
				DestinationAddressPrefix: to.StringPtr(primaryNetworkAddress.Value),
				Access:    network.SecurityRuleAccessAllow,
				Priority:  to.Int32Ptr(priority),
				Direction: network.SecurityRuleDirectionInbound,
			},
		}
		_, errCh := securityRuleClient.CreateOrUpdate(
			inst.env.resourceGroup, securityGroupName, ruleName, securityRule,
			nil, // abort channel
		)
		if err := <-errCh; err != nil {
			return errors.Annotatef(err, "creating security rule for %s", securityRule)
		}
		securityRules = append(securityRules, securityRule)
	}
	return nil
}

// ClosePorts is specified in the Instance interface.
func (inst *azureInstance) ClosePorts(ctx context.ProviderCallContext, machineId string, rules []jujunetwork.IngressRule) error {
	securityRuleClient := network.SecurityRulesClient{inst.env.network}
	securityGroupName := internalSecurityGroupName

	// Delete rules one at a time; this is necessary to avoid trampling
	// on changes made by the provisioner.
	vmName := resourceName(names.NewMachineTag(machineId))
	prefix := instanceNetworkSecurityRulePrefix(instance.Id(vmName))

	singleSourceIngressRules := explodeIngressRules(rules)
	for _, rule := range singleSourceIngressRules {
		ruleName := securityRuleName(prefix, rule)
		logger.Debugf("deleting security rule %q", ruleName)
		resultCh, errCh := securityRuleClient.Delete(
			inst.env.resourceGroup, securityGroupName, ruleName,
			nil, // abort channel
		)
		result, err := <-resultCh, <-errCh
		if err != nil && !isNotFoundResponse(result) {
			return errors.Annotatef(err, "deleting security rule %q", ruleName)
		}
	}
	return nil
}

// IngressRules is specified in the Instance interface.
func (inst *azureInstance) IngressRules(ctx context.ProviderCallContext, machineId string) (rules []jujunetwork.IngressRule, err error) {
	nsgClient := network.SecurityGroupsClient{inst.env.network}
	securityGroupName := internalSecurityGroupName
	nsg, err := nsgClient.Get(inst.env.resourceGroup, securityGroupName, "")
	if err != nil {
		return nil, errors.Annotate(err, "querying network security group")
	}
	if nsg.SecurityRules == nil {
		return nil, nil
	}

	vmName := resourceName(names.NewMachineTag(machineId))
	prefix := instanceNetworkSecurityRulePrefix(instance.Id(vmName))

	// Keep track of all the SourceAddressPrefixes for each port range.
	portSourceCIDRs := make(map[jujunetwork.PortRange]*[]string)
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

		var portRange jujunetwork.PortRange
		if *rule.DestinationPortRange == "*" {
			portRange.FromPort = 0
			portRange.ToPort = 65535
		} else {
			portRange, err = jujunetwork.ParsePortRange(
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
		rule, err := jujunetwork.NewIngressRule(
			portRange.Protocol,
			portRange.FromPort,
			portRange.ToPort,
			*sourceCIDRs...)
		if err != nil {
			return nil, errors.Trace(err)
		}
		rules = append(rules, rule)
	}
	jujunetwork.SortIngressRules(rules)
	return rules, nil
}

// deleteInstanceNetworkSecurityRules deletes network security rules in the
// internal network security group that correspond to the specified machine.
//
// This is expected to delete *all* security rules related to the instance,
// i.e. both the ones opened by OpenPorts above, and the ones opened for API
// access.
func deleteInstanceNetworkSecurityRules(
	resourceGroup string, id instance.Id,
	nsgClient network.SecurityGroupsClient,
	securityRuleClient network.SecurityRulesClient,
) error {
	nsg, err := nsgClient.Get(resourceGroup, internalSecurityGroupName, "")
	if err != nil {
		if err2, ok := err.(autorest.DetailedError); ok && err2.Response.StatusCode == http.StatusNotFound {
			return nil
		}
		return errors.Annotate(err, "querying network security group")
	}
	if nsg.SecurityRules == nil {
		return nil
	}
	prefix := instanceNetworkSecurityRulePrefix(id)
	for _, rule := range *nsg.SecurityRules {
		ruleName := to.String(rule.Name)
		if !strings.HasPrefix(ruleName, prefix) {
			continue
		}
		resultCh, errCh := securityRuleClient.Delete(
			resourceGroup,
			internalSecurityGroupName,
			ruleName,
			nil,
		)
		result, err := <-resultCh, <-errCh
		if err != nil && !isNotFoundResponse(result) {
			return errors.Annotatef(err, "deleting security rule %q", ruleName)
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
func securityRuleName(prefix string, rule jujunetwork.IngressRule) string {
	ruleName := fmt.Sprintf("%s%s-%d", prefix, rule.Protocol, rule.FromPort)
	if rule.FromPort != rule.ToPort {
		ruleName += fmt.Sprintf("-%d", rule.ToPort)
	}
	// The rule parameter must have a single source cidr.
	// Ensure the rule name can be a valid URL path component.
	cidr := rule.SourceCIDRs[0]
	if cidr != "0.0.0.0/0" && cidr != "*" {
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
func explodeIngressRules(inRules jujunetwork.IngressRuleSlice) jujunetwork.IngressRuleSlice {
	// If any rule has an empty source CIDR slice, a default
	// source value of "*" is used.
	var singleSourceIngressRules jujunetwork.IngressRuleSlice
	for _, rule := range inRules {
		sourceCIDRs := rule.SourceCIDRs
		if len(sourceCIDRs) == 0 {
			sourceCIDRs = []string{"*"}
		}
		for _, sr := range sourceCIDRs {
			r := rule
			r.SourceCIDRs = []string{sr}
			singleSourceIngressRules = append(singleSourceIngressRules, r)
		}
	}
	return singleSourceIngressRules
}
