// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package azure

import (
	"fmt"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/network/armnetwork"
	"github.com/juju/errors"

	"github.com/juju/juju/internal/provider/azure/internal/armtemplates"
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

const (
	apiSecurityRulePrefix = "JujuAPIInbound"
	sshSecurityRuleName   = "SSHInbound"
)

// newSecurityRule returns a security rule with the given parameters.
func newSecurityRule(p newSecurityRuleParams) *armnetwork.SecurityRule {
	return &armnetwork.SecurityRule{
		Name: to.Ptr(p.name),
		Properties: &armnetwork.SecurityRulePropertiesFormat{
			Description:              to.Ptr(p.description),
			Protocol:                 to.Ptr(armnetwork.SecurityRuleProtocolTCP),
			SourceAddressPrefix:      to.Ptr("*"),
			SourcePortRange:          to.Ptr("*"),
			DestinationAddressPrefix: to.Ptr(p.destPrefix),
			DestinationPortRange:     to.Ptr(fmt.Sprint(p.port)),
			Access:                   to.Ptr(armnetwork.SecurityRuleAccessAllow),
			Priority:                 to.Ptr(int32(p.priority)),
			Direction:                to.Ptr(armnetwork.SecurityRuleDirectionInbound),
		},
	}
}

// newSecurityRuleParams holds parameters for calling newSecurityRule, like the
// rule name, description, the destination address prefix, port and priority.
type newSecurityRuleParams struct {
	name        string
	description string
	destPrefix  string
	port        int
	priority    int
}

// networkTemplateResources returns resource definitions for creating network
// resources shared by all machines in a model.
//
// If apiPort is -1, then there should be no controller subnet created, and
// no network security rule allowing Juju API traffic.
func networkTemplateResources(
	location string,
	envTags map[string]string,
	apiPorts []int,
	extraRules []*armnetwork.SecurityRule,
) ([]armtemplates.Resource, []string) {
	securityRules := networkSecurityRules(apiPorts, extraRules)
	nsgID := fmt.Sprintf(
		`[resourceId('Microsoft.Network/networkSecurityGroups', '%s')]`,
		internalSecurityGroupName,
	)
	resources := []armtemplates.Resource{{
		APIVersion: networkAPIVersion,
		Type:       "Microsoft.Network/networkSecurityGroups",
		Name:       internalSecurityGroupName,
		Location:   location,
		Tags:       envTags,
		Properties: &armnetwork.SecurityGroupPropertiesFormat{
			SecurityRules: securityRules,
		},
	}}
	subnets := []*armnetwork.Subnet{{
		Name: to.Ptr(internalSubnetName),
		Properties: &armnetwork.SubnetPropertiesFormat{
			AddressPrefix: to.Ptr(internalSubnetPrefix),
			NetworkSecurityGroup: &armnetwork.SecurityGroup{
				ID: to.Ptr(nsgID),
			},
		},
	}}
	addressPrefixes := []*string{to.Ptr(internalSubnetPrefix)}
	if len(apiPorts) > 0 {
		addressPrefixes = append(addressPrefixes, to.Ptr(controllerSubnetPrefix))
		subnets = append(subnets, &armnetwork.Subnet{
			Name: to.Ptr(controllerSubnetName),
			Properties: &armnetwork.SubnetPropertiesFormat{
				AddressPrefix: to.Ptr(controllerSubnetPrefix),
				NetworkSecurityGroup: &armnetwork.SecurityGroup{
					ID: to.Ptr(nsgID),
				},
			},
		})
	}
	resources = append(resources, armtemplates.Resource{
		APIVersion: networkAPIVersion,
		Type:       "Microsoft.Network/virtualNetworks",
		Name:       internalNetworkName,
		Location:   location,
		Tags:       envTags,
		Properties: &armnetwork.VirtualNetworkPropertiesFormat{
			AddressSpace: &armnetwork.AddressSpace{addressPrefixes},
			Subnets:      subnets,
		},
		DependsOn: []string{nsgID},
	})
	return resources, []string{nsgID}
}

// networkSecurityRules creates network security rules for the environment.
func networkSecurityRules(
	apiPorts []int,
	extraRules []*armnetwork.SecurityRule,
) []*armnetwork.SecurityRule {
	securityRules := []*armnetwork.SecurityRule{newSecurityRule(newSecurityRuleParams{
		name:        sshSecurityRuleName,
		description: "Allow SSH access to all machines",
		destPrefix:  "*",
		port:        22,
		priority:    securityRuleInternalSSHInbound,
	})}
	for i, apiPort := range apiPorts {
		securityRules = append(securityRules, newSecurityRule(newSecurityRuleParams{
			// Two rules cannot have the same name.
			name:        fmt.Sprintf("%s%d", apiSecurityRulePrefix, apiPort),
			description: "Allow API connections to controller machines",
			destPrefix:  controllerSubnetPrefix,
			port:        apiPort,
			// Two rules cannot have the same priority and direction.
			priority: securityRuleInternalAPIInbound + i,
		}))
	}
	securityRules = append(securityRules, extraRules...)
	return securityRules
}

// nextSecurityRulePriority returns the next available priority in the given
// security group within a specified range.
func nextSecurityRulePriority(group *armnetwork.SecurityGroup, min, max int32) (int32, error) {
	if group.Properties == nil {
		return min, nil
	}
	for p := min; p <= max; p++ {
		var found bool
		for _, rule := range group.Properties.SecurityRules {
			if rule.Properties == nil {
				continue
			}
			if toValue(rule.Properties.Priority) == p {
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
