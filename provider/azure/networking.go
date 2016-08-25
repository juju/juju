// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package azure

import (
	"fmt"
	"net"
	"path"

	"github.com/Azure/azure-sdk-for-go/arm/compute"
	"github.com/Azure/azure-sdk-for-go/arm/network"
	"github.com/Azure/go-autorest/autorest"
	"github.com/Azure/go-autorest/autorest/to"
	"github.com/juju/errors"

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

func createInternalVirtualNetwork(
	callAPI callAPIFunc,
	client network.ManagementClient,
	subscriptionId, resourceGroup string,
	location string,
	tags map[string]string,
) (*network.VirtualNetwork, error) {
	addressPrefixes := []string{internalSubnetPrefix, controllerSubnetPrefix}
	vnet := network.VirtualNetwork{
		Location: to.StringPtr(location),
		Tags:     to.StringMapPtr(tags),
		Properties: &network.VirtualNetworkPropertiesFormat{
			AddressSpace: &network.AddressSpace{&addressPrefixes},
		},
	}
	logger.Debugf("creating virtual network %q", internalNetworkName)
	vnetClient := network.VirtualNetworksClient{client}
	if err := callAPI(func() (autorest.Response, error) {
		return vnetClient.CreateOrUpdate(
			resourceGroup, internalNetworkName, vnet,
			nil, // abort channel
		)
	}); err != nil {
		return nil, errors.Annotatef(err, "creating virtual network %q", internalNetworkName)
	}
	vnet.ID = to.StringPtr(internalNetworkId(subscriptionId, resourceGroup))
	return &vnet, nil
}

func createInternalNetworkSecurityGroup(
	callAPI callAPIFunc,
	client network.ManagementClient,
	subscriptionId, resourceGroup string,
	location string,
	tags map[string]string,
	apiPort *int,
) (*network.SecurityGroup, error) {
	// Create a network security group for the environment. There is only
	// one NSG per environment (there's a limit of 100 per subscription),
	// in which we manage rules for each exposed machine.
	securityRules := []network.SecurityRule{sshSecurityRule}
	if apiPort != nil {
		rule := apiSecurityRule
		properties := *rule.Properties
		properties.DestinationPortRange = to.StringPtr(fmt.Sprint(*apiPort))
		rule.Properties = &properties
		securityRules = append(securityRules, rule)
	}
	nsg := network.SecurityGroup{
		Location: to.StringPtr(location),
		Tags:     to.StringMapPtr(tags),
		Properties: &network.SecurityGroupPropertiesFormat{
			SecurityRules: &securityRules,
		},
	}
	securityGroupClient := network.SecurityGroupsClient{client}
	securityGroupName := internalSecurityGroupName
	logger.Debugf("creating security group %q", securityGroupName)
	if err := callAPI(func() (autorest.Response, error) {
		return securityGroupClient.CreateOrUpdate(
			resourceGroup, securityGroupName, nsg,
			nil, // abort channel
		)
	}); err != nil {
		return nil, errors.Annotatef(err, "creating security group %q", securityGroupName)
	}
	nsg.ID = to.StringPtr(internalNetworkSecurityGroupId(subscriptionId, resourceGroup))
	return &nsg, nil
}

// createInternalSubnet creates an internal subnet for the specified resource group,
// within the specified virtual network.
//
// NOTE(axw) this method expects an up-to-date VirtualNetwork, and expects that are
// no concurrent subnet additions to the virtual network. At the moment we have only
// three places where we modify subnets: at bootstrap, when a new environment is
// created, and when an environment is destroyed.
func createInternalNetworkSubnet(
	callAPI callAPIFunc,
	client network.ManagementClient,
	subscriptionId, resourceGroup string,
	subnetName, addressPrefix string,
	location string,
	tags map[string]string,
) (*network.Subnet, error) {
	// Now create a subnet with the next available address prefix, and
	// associate the subnet with the NSG created above.
	subnet := network.Subnet{
		Properties: &network.SubnetPropertiesFormat{
			AddressPrefix: to.StringPtr(addressPrefix),
			NetworkSecurityGroup: &network.SecurityGroup{
				ID: to.StringPtr(internalNetworkSecurityGroupId(
					subscriptionId, resourceGroup,
				)),
			},
		},
	}
	logger.Debugf("creating subnet %q (%s)", subnetName, addressPrefix)
	subnetClient := network.SubnetsClient{client}
	if err := callAPI(func() (autorest.Response, error) {
		return subnetClient.CreateOrUpdate(
			resourceGroup, internalNetworkName, subnetName, subnet,
			nil, // abort channel
		)
	}); err != nil {
		return nil, errors.Annotatef(err, "creating subnet %q", subnetName)
	}
	subnet.ID = to.StringPtr(internalNetworkSubnetId(
		subscriptionId, resourceGroup, subnetName,
	))
	return &subnet, nil
}

type subnetParams struct {
	name   string
	prefix string
}

// newNetworkProfile creates a public IP and NIC(s) for the VM with the
// specified name. A separate NIC will be created for each subnet; the
// first subnet in the list will be associated with the primary NIC.
func newNetworkProfile(
	callAPI callAPIFunc,
	client network.ManagementClient,
	vmName string,
	controller bool,
	subscriptionId, resourceGroup string,
	location string,
	tags map[string]string,
) (*compute.NetworkProfile, error) {
	logger.Debugf("creating network profile for %q", vmName)

	// Create a public IP for the NIC. Public IP addresses are dynamic.
	logger.Debugf("- allocating public IP address")
	pipClient := network.PublicIPAddressesClient{client}
	publicIPAddress := network.PublicIPAddress{
		Location: to.StringPtr(location),
		Tags:     to.StringMapPtr(tags),
		Properties: &network.PublicIPAddressPropertiesFormat{
			PublicIPAllocationMethod: network.Dynamic,
		},
	}
	publicIPAddressName := vmName + "-public-ip"
	if err := callAPI(func() (autorest.Response, error) {
		return pipClient.CreateOrUpdate(
			resourceGroup, publicIPAddressName, publicIPAddress,
			nil, // abort channel
		)
	}); err != nil {
		return nil, errors.Annotatef(err, "creating public IP address for %q", vmName)
	}
	publicIPAddress.ID = to.StringPtr(publicIPAddressId(
		subscriptionId, resourceGroup, publicIPAddressName,
	))

	// Controller and non-controller machines are assigned to separate
	// subnets. This enables us to create controller-specific NSG rules
	// just by targeting the controller subnet.
	subnetName := internalSubnetName
	subnetPrefix := internalSubnetPrefix
	if controller {
		subnetName = controllerSubnetName
		subnetPrefix = controllerSubnetPrefix
	}
	subnetId := internalNetworkSubnetId(subscriptionId, resourceGroup, subnetName)

	// Determine the next available private IP address.
	nicClient := network.InterfacesClient{client}
	privateIPAddress, err := nextSubnetIPAddress(nicClient, resourceGroup, subnetPrefix)
	if err != nil {
		return nil, errors.Annotatef(err, "querying private IP addresses")
	}

	// Create a primary NIC for the machine. The private IP address needs
	// to be static so that we can create NSG rules that don't become
	// invalid.
	logger.Debugf("- creating primary NIC")
	ipConfigurations := []network.InterfaceIPConfiguration{{
		Name: to.StringPtr("primary"),
		Properties: &network.InterfaceIPConfigurationPropertiesFormat{
			Primary:                   to.BoolPtr(true),
			PrivateIPAddress:          to.StringPtr(privateIPAddress),
			PrivateIPAllocationMethod: network.Static,
			Subnet: &network.Subnet{ID: to.StringPtr(subnetId)},
			PublicIPAddress: &network.PublicIPAddress{
				ID: publicIPAddress.ID,
			},
		},
	}}
	nicName := vmName + "-primary"
	nic := network.Interface{
		Location: to.StringPtr(location),
		Tags:     to.StringMapPtr(tags),
		Properties: &network.InterfacePropertiesFormat{
			IPConfigurations: &ipConfigurations,
		},
	}
	if err := callAPI(func() (autorest.Response, error) {
		return nicClient.CreateOrUpdate(resourceGroup, nicName, nic, nil)
	}); err != nil {
		return nil, errors.Annotatef(err, "creating network interface for %q", vmName)
	}

	nics := []compute.NetworkInterfaceReference{{
		ID: to.StringPtr(networkInterfaceId(subscriptionId, resourceGroup, nicName)),
		Properties: &compute.NetworkInterfaceReferenceProperties{
			Primary: to.BoolPtr(true),
		},
	}}
	return &compute.NetworkProfile{&nics}, nil
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

// nextSubnetIPAddress returns the next available IP address in the given subnet.
func nextSubnetIPAddress(
	nicClient network.InterfacesClient,
	resourceGroup string,
	subnetPrefix string,
) (string, error) {
	_, ipnet, err := net.ParseCIDR(subnetPrefix)
	if err != nil {
		return "", errors.Annotate(err, "parsing subnet prefix")
	}
	results, err := nicClient.List(resourceGroup)
	if err != nil {
		return "", errors.Annotate(err, "listing NICs")
	}
	var ipsInUse []net.IP
	if results.Value != nil {
		ipsInUse = make([]net.IP, 0, len(*results.Value))
		for _, item := range *results.Value {
			if item.Properties.IPConfigurations == nil {
				continue
			}
			for _, ipConfiguration := range *item.Properties.IPConfigurations {
				ip := net.ParseIP(to.String(ipConfiguration.Properties.PrivateIPAddress))
				if ip != nil && ipnet.Contains(ip) {
					ipsInUse = append(ipsInUse, ip)
				}
			}
		}
	}
	ip, err := iputils.NextSubnetIP(ipnet, ipsInUse)
	if err != nil {
		return "", errors.Trace(err)
	}
	return ip.String(), nil
}

// internalNetworkSubnetId returns the Azure resource ID of the subnet with
// the specified name, within the internal network subnet for the specified
// resource group.
func internalNetworkSubnetId(subscriptionId, resourceGroup, subnetName string) string {
	return path.Join(
		internalNetworkId(subscriptionId, resourceGroup),
		"subnets", subnetName,
	)
}

func internalNetworkId(subscriptionId, resourceGroup string) string {
	return resourceId(subscriptionId, resourceGroup, "Microsoft.Network",
		"virtualNetworks", internalNetworkName,
	)
}

func internalNetworkSecurityGroupId(subscriptionId, resourceGroup string) string {
	return resourceId(subscriptionId, resourceGroup, "Microsoft.Network",
		"networkSecurityGroups", internalSecurityGroupName,
	)
}

func publicIPAddressId(subscriptionId, resourceGroup, publicIPAddressName string) string {
	return resourceId(subscriptionId, resourceGroup, "Microsoft.Network",
		"publicIPAddresses", publicIPAddressName,
	)
}

func networkInterfaceId(subscriptionId, resourceGroup, interfaceName string) string {
	return resourceId(subscriptionId, resourceGroup, "Microsoft.Network",
		"networkInterfaces", interfaceName,
	)
}

func resourceId(subscriptionId, resourceGroup, provider string, resourceId ...string) string {
	args := append([]string{
		"/subscriptions", subscriptionId,
		"resourceGroups", resourceGroup,
		"providers", provider,
	}, resourceId...)
	return path.Join(args...)
}
