// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package azure

import (
	"fmt"
	"net"
	"strconv"

	"github.com/Azure/azure-sdk-for-go/arm/network"
	"github.com/Azure/go-autorest/autorest/to"
	"github.com/juju/errors"

	"github.com/juju/juju/provider/azure/internal/armtemplates"
	"github.com/juju/juju/provider/azure/internal/iputils"
)

const (
	// internalNetworkName is the name of the virtual network that all
	// Juju machines within a resource group are connected to.
	//
	// Each resource group is given its own network, subnet and network
	// security group to manage. Each resource group will have its own
	// private 192.168.0.0/16 network.
	internalNetworkName = "juju-internal-network"

	// internalSecurityGroupName is the name of the network security
	// group that each machine's primary (internal network) NIC is
	// attached to.
	internalSecurityGroupName = "juju-internal-nsg"

	// internalSubnetName is the name of the subnet that each
	// non-controller machine's primary NIC is attached to.
	internalSubnetName = "juju-internal-subnet"

	// internalSubnetPrefix is the address prefix for the subnet that
	// each non-controller machine's primary NIC is attached to.
	internalSubnetPrefix = "192.168.0.0/20"

	// controllerSubnetName is the name of the subnet that each controller
	// machine's primary NIC is attached to.
	controllerSubnetName = "juju-controller-subnet"

	// controllerSubnetPrefix is the address prefix for the subnet that
	// each controller machine's primary NIC is attached to.
	controllerSubnetPrefix = "192.168.16.0/20"
)

const (
	// securityRuleInternalMin is the beginning of the range of
	// internal security group rules defined by Juju.
	securityRuleInternalMin = 100

	// securityRuleInternalMax is the end of the range of internal
	// security group rules defined by Juju.
	securityRuleInternalMax = 199

	// securityRuleMax is the maximum allowable security rule
	// priority.
	securityRuleMax = 4096
)

const (
	// securityRuleInternalSSHInbound is the priority of the
	// security rule that allows inbound SSH access to all
	// machines.
	securityRuleInternalSSHInbound = securityRuleInternalMin + iota

	// securityRuleInternalAPIInbound is the priority of the
	// security rule that allows inbound Juju API access to
	// controller machines
	securityRuleInternalAPIInbound
)

var (
	sshSecurityRule = network.SecurityRule{
		Name: to.StringPtr("SSHInbound"),
		Properties: &network.SecurityRulePropertiesFormat{
			Description:              to.StringPtr("Allow SSH access to all machines"),
			Protocol:                 network.TCP,
			SourceAddressPrefix:      to.StringPtr("*"),
			SourcePortRange:          to.StringPtr("*"),
			DestinationAddressPrefix: to.StringPtr("*"),
			DestinationPortRange:     to.StringPtr("22"),
			Access:                   network.Allow,
			Priority:                 to.Int32Ptr(securityRuleInternalSSHInbound),
			Direction:                network.Inbound,
		},
	}

	apiSecurityRule = network.SecurityRule{
		Name: to.StringPtr("JujuAPIInbound"),
		Properties: &network.SecurityRulePropertiesFormat{
			Description:              to.StringPtr("Allow API connections to controller machines"),
			Protocol:                 network.TCP,
			SourceAddressPrefix:      to.StringPtr("*"),
			SourcePortRange:          to.StringPtr("*"),
			DestinationAddressPrefix: to.StringPtr(controllerSubnetPrefix),
			// DestinationPortRange is set by createInternalNetworkSecurityGroup.
			Access:    network.Allow,
			Priority:  to.Int32Ptr(securityRuleInternalAPIInbound),
			Direction: network.Inbound,
		},
	}
)

// networkTemplateResources returns resource definitions for creating network
// resources shared by all machines in a model.
func networkTemplateResources(
	location string,
	envTags map[string]string,
	apiPort int,
) []armtemplates.Resource {
	// Create a network security group for the environment. There is only
	// one NSG per environment (there's a limit of 100 per subscription),
	// in which we manage rules for each exposed machine.
	apiSecurityRule := apiSecurityRule
	properties := *apiSecurityRule.Properties
	properties.DestinationPortRange = to.StringPtr(fmt.Sprint(apiPort))
	apiSecurityRule.Properties = &properties
	securityRules := []network.SecurityRule{sshSecurityRule, apiSecurityRule}

	// NOTE(axw) we create the API rule for all models to avoid having to
	// make queries when creating resources, making deployment faster and
	// more robust. The controller subnet is never used in non-controller
	// models, so there are no security implications.
	nsgId := fmt.Sprintf(
		`[resourceId('Microsoft.Network/networkSecurityGroups', '%s')]`,
		internalSecurityGroupName,
	)
	subnets := []network.Subnet{{
		Name: to.StringPtr(internalSubnetName),
		Properties: &network.SubnetPropertiesFormat{
			AddressPrefix: to.StringPtr(internalSubnetPrefix),
			NetworkSecurityGroup: &network.SecurityGroup{
				ID: to.StringPtr(nsgId),
			},
		},
	}, {
		Name: to.StringPtr(controllerSubnetName),
		Properties: &network.SubnetPropertiesFormat{
			AddressPrefix: to.StringPtr(controllerSubnetPrefix),
			NetworkSecurityGroup: &network.SecurityGroup{
				ID: to.StringPtr(nsgId),
			},
		},
	}}

	addressPrefixes := []string{internalSubnetPrefix, controllerSubnetPrefix}
	resources := []armtemplates.Resource{{
		APIVersion: network.APIVersion,
		Type:       "Microsoft.Network/networkSecurityGroups",
		Name:       internalSecurityGroupName,
		Location:   location,
		Tags:       envTags,
		Properties: &network.SecurityGroupPropertiesFormat{
			SecurityRules: &securityRules,
		},
	}, {
		APIVersion: network.APIVersion,
		Type:       "Microsoft.Network/virtualNetworks",
		Name:       internalNetworkName,
		Location:   location,
		Tags:       envTags,
		Properties: &network.VirtualNetworkPropertiesFormat{
			AddressSpace: &network.AddressSpace{&addressPrefixes},
			Subnets:      &subnets,
		},
		DependsOn: []string{nsgId},
	}}
	return resources
}

// nextSecurityRulePriority returns the next available priority in the given
// security group within a specified range.
func nextSecurityRulePriority(group network.SecurityGroup, min, max int32) (int32, error) {
	if group.Properties.SecurityRules == nil {
		return min, nil
	}
	for p := min; p <= max; p++ {
		var found bool
		for _, rule := range *group.Properties.SecurityRules {
			if to.Int32(rule.Properties.Priority) == p {
				found = true
				break
			}
		}
		if !found {
			return p, nil
		}
	}
	return -1, errors.Errorf(
		"no priorities available in the range [%d, %d]", min, max,
	)
}

// machineSubnetIP returns the private IP address to use for the given
// subnet prefix.
func machineSubnetIP(subnetPrefix, machineId string) (net.IP, error) {
	_, ipnet, err := net.ParseCIDR(subnetPrefix)
	if err != nil {
		return nil, errors.Annotate(err, "parsing subnet prefix")
	}
	n, err := strconv.Atoi(machineId)
	if err != nil {
		return nil, errors.Annotate(err, "parsing machine ID")
	}
	ip := iputils.NthSubnetIP(ipnet, n)
	if ip == nil {
		// TODO(axw) getting nil means we've cycled through roughly
		// 2^12 machines. To work around this limitation, we must
		// maintain an in-memory set of in-use IP addresses for each
		// subnet.
		return nil, errors.Errorf(
			"no available IP addresses in %s", subnetPrefix,
		)
	}
	return ip, nil
}
