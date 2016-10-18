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
	"github.com/juju/juju/instance"
	jujunetwork "github.com/juju/juju/network"
	"github.com/juju/juju/status"
	"gopkg.in/juju/names.v2"
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
func (inst *azureInstance) Status() instance.InstanceStatus {
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
	callAPI callAPIFunc,
	resourceGroup string,
	nicClient network.InterfacesClient,
	pipClient network.PublicIPAddressesClient,
	instances []*azureInstance,
) (err error) {
	instanceNics, err := instanceNetworkInterfaces(
		callAPI, resourceGroup, nicClient,
	)
	if err != nil {
		return errors.Annotate(err, "listing network interfaces")
	}
	instancePips, err := instancePublicIPAddresses(
		callAPI, resourceGroup, pipClient,
	)
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
	callAPI callAPIFunc,
	resourceGroup string,
	nicClient network.InterfacesClient,
) (map[instance.Id][]network.Interface, error) {
	var nicsResult network.InterfaceListResult
	if err := callAPI(func() (autorest.Response, error) {
		var err error
		nicsResult, err = nicClient.List(resourceGroup)
		return nicsResult.Response, err
	}); err != nil {
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
	callAPI callAPIFunc,
	resourceGroup string,
	pipClient network.PublicIPAddressesClient,
) (map[instance.Id][]network.PublicIPAddress, error) {
	var pipsResult network.PublicIPAddressListResult
	if err := callAPI(func() (autorest.Response, error) {
		var err error
		pipsResult, err = pipClient.List(resourceGroup)
		return pipsResult.Response, err
	}); err != nil {
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
func (inst *azureInstance) Addresses() ([]jujunetwork.Address, error) {
	addresses := make([]jujunetwork.Address, 0, len(inst.networkInterfaces)+len(inst.publicIPAddresses))
	for _, nic := range inst.networkInterfaces {
		if nic.Properties.IPConfigurations == nil {
			continue
		}
		for _, ipConfiguration := range *nic.Properties.IPConfigurations {
			privateIpAddress := ipConfiguration.Properties.PrivateIPAddress
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
		if pip.Properties.IPAddress == nil {
			continue
		}
		addresses = append(addresses, jujunetwork.NewScopedAddress(
			to.String(pip.Properties.IPAddress),
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
		if nic.Properties.IPConfigurations == nil {
			continue
		}
		for _, ipConfiguration := range *nic.Properties.IPConfigurations {
			if ipConfiguration.Properties.Subnet == nil {
				continue
			}
			if !to.Bool(ipConfiguration.Properties.Primary) {
				continue
			}
			privateIpAddress := ipConfiguration.Properties.PrivateIPAddress
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
func (inst *azureInstance) OpenPorts(machineId string, ports []jujunetwork.PortRange) error {
	nsgClient := network.SecurityGroupsClient{inst.env.network}
	securityRuleClient := network.SecurityRulesClient{inst.env.network}
	primaryNetworkAddress, err := inst.primaryNetworkAddress()
	if err != nil {
		return errors.Trace(err)
	}

	securityGroupName := internalSecurityGroupName
	var nsg network.SecurityGroup
	if err := inst.env.callAPI(func() (autorest.Response, error) {
		var err error
		nsg, err = nsgClient.Get(inst.env.resourceGroup, securityGroupName, "")
		return nsg.Response, err
	}); err != nil {
		return errors.Annotate(err, "querying network security group")
	}

	var securityRules []network.SecurityRule
	if nsg.Properties.SecurityRules != nil {
		securityRules = *nsg.Properties.SecurityRules
	} else {
		nsg.Properties.SecurityRules = &securityRules
	}

	// Create rules one at a time; this is necessary to avoid trampling
	// on changes made by the provisioner. We still record rules in the
	// NSG in memory, so we can easily tell which priorities are available.
	vmName := resourceName(names.NewMachineTag(machineId))
	prefix := instanceNetworkSecurityRulePrefix(instance.Id(vmName))
	for _, ports := range ports {
		ruleName := securityRuleName(prefix, ports)

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
			return errors.Annotatef(err, "getting security rule priority for %s", ports)
		}

		var protocol network.SecurityRuleProtocol
		switch ports.Protocol {
		case "tcp":
			protocol = network.TCP
		case "udp":
			protocol = network.UDP
		default:
			return errors.Errorf("invalid protocol %q", ports.Protocol)
		}

		var portRange string
		if ports.FromPort != ports.ToPort {
			portRange = fmt.Sprintf("%d-%d", ports.FromPort, ports.ToPort)
		} else {
			portRange = fmt.Sprint(ports.FromPort)
		}

		rule := network.SecurityRule{
			Properties: &network.SecurityRulePropertiesFormat{
				Description:              to.StringPtr(ports.String()),
				Protocol:                 protocol,
				SourcePortRange:          to.StringPtr("*"),
				DestinationPortRange:     to.StringPtr(portRange),
				SourceAddressPrefix:      to.StringPtr("*"),
				DestinationAddressPrefix: to.StringPtr(primaryNetworkAddress.Value),
				Access:    network.Allow,
				Priority:  to.Int32Ptr(priority),
				Direction: network.Inbound,
			},
		}
		if err := inst.env.callAPI(func() (autorest.Response, error) {
			return securityRuleClient.CreateOrUpdate(
				inst.env.resourceGroup, securityGroupName, ruleName, rule,
				nil, // abort channel
			)
		}); err != nil {
			return errors.Annotatef(err, "creating security rule for %s", ports)
		}
		securityRules = append(securityRules, rule)
	}
	return nil
}

// ClosePorts is specified in the Instance interface.
func (inst *azureInstance) ClosePorts(machineId string, ports []jujunetwork.PortRange) error {
	securityRuleClient := network.SecurityRulesClient{inst.env.network}
	securityGroupName := internalSecurityGroupName

	// Delete rules one at a time; this is necessary to avoid trampling
	// on changes made by the provisioner.
	vmName := resourceName(names.NewMachineTag(machineId))
	prefix := instanceNetworkSecurityRulePrefix(instance.Id(vmName))
	for _, ports := range ports {
		ruleName := securityRuleName(prefix, ports)
		logger.Debugf("deleting security rule %q", ruleName)
		var result autorest.Response
		if err := inst.env.callAPI(func() (autorest.Response, error) {
			var err error
			result, err = securityRuleClient.Delete(
				inst.env.resourceGroup, securityGroupName, ruleName,
				nil, // abort channel
			)
			return result, err
		}); err != nil {
			if result.Response == nil || result.StatusCode != http.StatusNotFound {
				return errors.Annotatef(err, "deleting security rule %q", ruleName)
			}
		}
	}
	return nil
}

// Ports is specified in the Instance interface.
func (inst *azureInstance) Ports(machineId string) (ports []jujunetwork.PortRange, err error) {
	nsgClient := network.SecurityGroupsClient{inst.env.network}
	securityGroupName := internalSecurityGroupName
	var nsg network.SecurityGroup
	if err := inst.env.callAPI(func() (autorest.Response, error) {
		var err error
		nsg, err = nsgClient.Get(inst.env.resourceGroup, securityGroupName, "")
		return nsg.Response, err
	}); err != nil {
		return nil, errors.Annotate(err, "querying network security group")
	}
	if nsg.Properties.SecurityRules == nil {
		return nil, nil
	}

	vmName := resourceName(names.NewMachineTag(machineId))
	prefix := instanceNetworkSecurityRulePrefix(instance.Id(vmName))
	for _, rule := range *nsg.Properties.SecurityRules {
		if rule.Properties.Direction != network.Inbound {
			continue
		}
		if rule.Properties.Access != network.Allow {
			continue
		}
		if to.Int32(rule.Properties.Priority) <= securityRuleInternalMax {
			continue
		}
		if !strings.HasPrefix(to.String(rule.Name), prefix) {
			continue
		}

		var portRange jujunetwork.PortRange
		if *rule.Properties.DestinationPortRange == "*" {
			portRange.FromPort = 0
			portRange.ToPort = 65535
		} else {
			portRange, err = jujunetwork.ParsePortRange(
				*rule.Properties.DestinationPortRange,
			)
			if err != nil {
				return nil, errors.Annotatef(
					err, "parsing port range for security rule %q",
					to.String(rule.Name),
				)
			}
		}

		var protocols []string
		switch rule.Properties.Protocol {
		case network.TCP:
			protocols = []string{"tcp"}
		case network.UDP:
			protocols = []string{"udp"}
		default:
			protocols = []string{"tcp", "udp"}
		}
		for _, protocol := range protocols {
			portRange.Protocol = protocol
			ports = append(ports, portRange)
		}
	}
	return ports, nil
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
	callAPI callAPIFunc,
) error {
	var nsg network.SecurityGroup
	if err := callAPI(func() (autorest.Response, error) {
		var err error
		nsg, err = nsgClient.Get(resourceGroup, internalSecurityGroupName, "")
		return nsg.Response, err
	}); err != nil {
		return errors.Annotate(err, "querying network security group")
	}
	if nsg.Properties.SecurityRules == nil {
		return nil
	}
	prefix := instanceNetworkSecurityRulePrefix(id)
	for _, rule := range *nsg.Properties.SecurityRules {
		ruleName := to.String(rule.Name)
		if !strings.HasPrefix(ruleName, prefix) {
			continue
		}
		var result autorest.Response
		err := callAPI(func() (autorest.Response, error) {
			var err error
			result, err = securityRuleClient.Delete(
				resourceGroup,
				internalSecurityGroupName,
				ruleName,
				nil, // abort channel
			)
			return result, err
		})
		if err != nil {
			if result.Response == nil || result.StatusCode != http.StatusNotFound {
				return errors.Annotatef(err, "deleting security rule %q", ruleName)
			}
		}
	}
	return nil
}

// instanceNetworkSecurityRulePrefix returns the unique prefix for network
// security rule names that relate to the instance with the given ID.
func instanceNetworkSecurityRulePrefix(id instance.Id) string {
	return string(id) + "-"
}

// securityRuleName returns the security rule name for the given port range,
// and prefix returned by instanceNetworkSecurityRulePrefix.
func securityRuleName(prefix string, ports jujunetwork.PortRange) string {
	ruleName := fmt.Sprintf("%s%s-%d", prefix, ports.Protocol, ports.FromPort)
	if ports.FromPort != ports.ToPort {
		ruleName += fmt.Sprintf("-%d", ports.ToPort)
	}
	return ruleName
}
