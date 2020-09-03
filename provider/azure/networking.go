// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package azure

import (
	"context"
	"fmt"

	"github.com/Azure/azure-sdk-for-go/services/network/mgmt/2018-08-01/network"
	"github.com/Azure/go-autorest/autorest/to"
	"github.com/juju/errors"

	"github.com/juju/juju/provider/azure/internal/armtemplates"
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
func newSecurityRule(p newSecurityRuleParams) network.SecurityRule {
	return network.SecurityRule{
		Name: to.StringPtr(p.name),
		SecurityRulePropertiesFormat: &network.SecurityRulePropertiesFormat{
			Description:              to.StringPtr(p.description),
			Protocol:                 network.SecurityRuleProtocolTCP,
			SourceAddressPrefix:      to.StringPtr("*"),
			SourcePortRange:          to.StringPtr("*"),
			DestinationAddressPrefix: to.StringPtr(p.destPrefix),
			DestinationPortRange:     to.StringPtr(fmt.Sprint(p.port)),
			Access:                   network.SecurityRuleAccessAllow,
			Priority:                 to.Int32Ptr(int32(p.priority)),
			Direction:                network.SecurityRuleDirectionInbound,
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
	config *azureModelConfig,
	envTags map[string]string,
	apiPorts []int,
	extraRules []network.SecurityRule,
) []armtemplates.Resource {
	// Create a network security group for the environment. There is only
	// one NSG per environment (there's a limit of 100 per subscription),
	// in which we manage rules for each exposed machine.
	securityRules := []network.SecurityRule{newSecurityRule(newSecurityRuleParams{
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

	nsgID := fmt.Sprintf(
		`[resourceId('Microsoft.Network/networkSecurityGroups', '%s')]`,
		internalSecurityGroupName,
	)
	subnets := []network.Subnet{{
		Name: to.StringPtr(internalSubnetName),
		SubnetPropertiesFormat: &network.SubnetPropertiesFormat{
			AddressPrefix: to.StringPtr(internalSubnetPrefix),
			NetworkSecurityGroup: &network.SecurityGroup{
				ID: to.StringPtr(nsgID),
			},
		},
	}}
	addressPrefixes := []string{internalSubnetPrefix}
	if len(apiPorts) > 0 {
		addressPrefixes = append(addressPrefixes, controllerSubnetPrefix)
		subnets = append(subnets, network.Subnet{
			Name: to.StringPtr(controllerSubnetName),
			SubnetPropertiesFormat: &network.SubnetPropertiesFormat{
				AddressPrefix: to.StringPtr(controllerSubnetPrefix),
				NetworkSecurityGroup: &network.SecurityGroup{
					ID: to.StringPtr(nsgID),
				},
			},
		})
	}

	resources := []armtemplates.Resource{{
		APIVersion: networkAPIVersion,
		Type:       "Microsoft.Network/networkSecurityGroups",
		Name:       internalSecurityGroupName,
		Location:   location,
		Tags:       envTags,
		Properties: &network.SecurityGroupPropertiesFormat{
			SecurityRules: &securityRules,
		},
	}, {
		APIVersion: networkAPIVersion,
		Type:       "Microsoft.Network/virtualNetworks",
		Name:       internalNetworkName,
		Location:   location,
		Tags:       envTags,
		Properties: &network.VirtualNetworkPropertiesFormat{
			AddressSpace: &network.AddressSpace{&addressPrefixes},
			Subnets:      &subnets,
		},
		DependsOn: []string{nsgID},
	}}

	return resources
}

// nextSecurityRulePriority returns the next available priority in the given
// security group within a specified range.
func nextSecurityRulePriority(group network.SecurityGroup, min, max int32) (int32, error) {
	if group.SecurityRules == nil {
		return min, nil
	}
	for p := min; p <= max; p++ {
		var found bool
		for _, rule := range *group.SecurityRules {
			if to.Int32(rule.Priority) == p {
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

// networkSecurityRules returns the network security rules for the internal
// network security group in the specified resource group. If the network
// security group has not been created, this function will return an error
// satisfying errors.IsNotFound.
func networkSecurityRules(
	nsgClient network.SecurityGroupsClient,
	resourceGroup string,
) ([]network.SecurityRule, error) {
	sdkCtx := context.Background()
	nsg, err := nsgClient.Get(sdkCtx, resourceGroup, internalSecurityGroupName, "")
	if err != nil {
		if isNotFoundResult(nsg.Response) {
			return nil, errors.NotFoundf("security group")
		}
		return nil, errors.Annotate(err, "querying network security group")
	}
	var rules []network.SecurityRule
	if nsg.SecurityRules != nil {
		rules = *nsg.SecurityRules
	}
	return rules, nil
}
