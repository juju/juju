// Copyright 2012-2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package openstack

import (
	"context"
	"fmt"
	"net"
	"strings"

	"github.com/go-goose/goose/v5/neutron"
	"github.com/go-goose/goose/v5/nova"
	"github.com/juju/collections/set"
	"github.com/juju/collections/transform"
	"github.com/juju/errors"
	"github.com/juju/utils/v4"

	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/environs"
)

type networkingBase struct {
	env *Environ
}

func (n networkingBase) client() NetworkingAuthenticatingClient {
	return n.env.client()
}

func (n networkingBase) neutron() NetworkingNeutron {
	return n.env.neutron()
}

func (n networkingBase) nova() NetworkingNova {
	return n.env.nova()
}

func (n networkingBase) ecfg() NetworkingEnvironConfig {
	return n.env.ecfg()
}

// NeutronNetworking is an implementation of Networking that uses the Neutron
// network APIs.
type NeutronNetworking struct {
	NetworkingBase
}

// projectIDFilter returns a neutron.Filter to match Neutron Networks with
// the given projectID.
func projectIDFilter(projectID string) *neutron.Filter {
	filter := neutron.NewFilter()
	filter.Set(neutron.FilterProjectId, projectID)
	return filter
}

// externalNetworkFilter returns a neutron.Filter to match Neutron Networks with
// router:external = true.
func externalNetworkFilter() *neutron.Filter {
	filter := neutron.NewFilter()
	filter.Set(neutron.FilterRouterExternal, "true")
	return filter
}

// internalNetworkFilter returns a neutron.Filter to match Neutron Networks with
// router:external = false.
func internalNetworkFilter() *neutron.Filter {
	filter := neutron.NewFilter()
	filter.Set(neutron.FilterRouterExternal, "false")
	return filter
}

// networkFilter returns a neutron.Filter to match Neutron Networks with
// the exact given name AND router:external boolean result.
func networkFilter(name string, external bool) *neutron.Filter {
	filter := neutron.NewFilter()
	filter.Set(neutron.FilterNetwork, fmt.Sprintf("%s", name))
	filter.Set(neutron.FilterRouterExternal, fmt.Sprintf("%t", external))
	return filter
}

func newNetworking(e *Environ) Networking {
	return &NeutronNetworking{NetworkingBase: networkingBase{env: e}}
}

// AllocatePublicIP is part of the Networking interface.
func (n *NeutronNetworking) AllocatePublicIP(id instance.Id) (*string, error) {
	// Look for external networks, in the same AZ as the server's networks,
	// to use for the FIP.
	detail, err := n.nova().GetServer(string(id))
	if err != nil {
		return nil, errors.Trace(err)
	}
	extNetworkIds, err := n.getExternalNetworkIDsFromHostAddrs(context.TODO(), detail.Addresses)
	if err != nil {
		return nil, errors.Trace(err)
	}

	// Look for FIPs in same project as the credentials.
	// Admins have visibility into other projects.
	fips, err := n.neutron().ListFloatingIPsV2(projectIDFilter(n.client().TenantId()))
	if err != nil {
		return nil, errors.Trace(err)
	}

	// Is there an unused FloatingIP on an external network
	// in the instance's availability zone?
	for _, fip := range fips {
		if fip.FixedIP == "" {
			// Not a perfect solution.  If an external network was specified in
			// the config, it'll be at the top of the extNetworkIds, but may be
			// not used if the available FIP isn't it in. However the instance
			// and the FIP will be in the same availability zone.
			for _, extNetId := range extNetworkIds {
				if fip.FloatingNetworkId == extNetId {
					logger.Debugf(context.TODO(), "found unassigned public ip: %v", fip.IP)
					return &fip.IP, nil
				}
			}
		}
	}

	// No unused FIPs exist, allocate a new IP and use it.
	var lastErr error
	for _, extNetId := range extNetworkIds {
		var newfip *neutron.FloatingIPV2
		newfip, lastErr = n.neutron().AllocateFloatingIPV2(extNetId)
		if lastErr == nil {
			logger.Debugf(context.TODO(), "allocated new public IP: %s", newfip.IP)
			return &newfip.IP, nil
		}
	}

	logger.Debugf(context.TODO(), "Unable to allocate a public IP")
	return nil, lastErr
}

// getExternalNetworkIDsFromHostAddrs returns a slice of external network IDs.
// If specified, the configured external network is returned. Otherwise search
// for an external network in the same availability zones as the provided
// server addresses.
func (n *NeutronNetworking) getExternalNetworkIDsFromHostAddrs(ctx context.Context, addrs map[string][]nova.IPAddress) ([]string, error) {
	var extNetworkIds []string
	externalNetwork := n.ecfg().externalNetwork()
	if externalNetwork != "" {
		// The config specified an external network, try it first.
		networks, err := n.ResolveNetworks(externalNetwork, true)
		if err != nil {
			logger.Warningf(ctx, "resolving configured external network %q: %s", externalNetwork, err.Error())
		} else {
			logger.Debugf(ctx, "using external network %q", externalNetwork)
			toID := func(n neutron.NetworkV2) string { return n.Id }
			extNetworkIds = transform.Slice(networks, toID)
		}
	}

	// We have a single external network ID, no need to search.
	if len(extNetworkIds) == 1 {
		return extNetworkIds, nil
	}

	logger.Debugf(ctx, "unique match for external network %q not found; searching for one", externalNetwork)

	hostAddrAZs, err := n.findNetworkAZForHostAddrs(addrs)
	if err != nil {
		return nil, errors.NewNotFound(nil,
			fmt.Sprintf("could not find an external network in availability zone(s) %q", hostAddrAZs.SortedValues()))
	}

	// Create slice of network.Ids for external networks in the same AZ as
	// the instance's networks, to find an existing floating ip in, or allocate
	// a new floating ip from.
	extNetIds, _ := getExternalNeutronNetworksByAZ(ctx, n, hostAddrAZs)

	// We have an external network ID, no need for specific error message.
	if len(extNetIds) > 0 {
		return extNetIds, nil
	}

	return nil, errors.NewNotFound(nil,
		fmt.Sprintf("could not find an external network in availability zone(s) %q", strings.Join(hostAddrAZs.SortedValues(), ", ")))
}

func (n *NeutronNetworking) findNetworkAZForHostAddrs(addrs map[string][]nova.IPAddress) (set.Strings, error) {
	netNames := set.NewStrings()
	for name := range addrs {
		netNames.Add(name)
	}
	if netNames.Size() == 0 {
		return nil, errors.NotFoundf("no networks found with server")
	}
	azNames := set.NewStrings()
	networks, err := n.neutron().ListNetworksV2(internalNetworkFilter())
	if err != nil {
		return nil, err
	}
	for _, network := range networks {
		if netNames.Contains(network.Name) {
			azNames = azNames.Union(set.NewStrings(network.AvailabilityZones...))
		}
	}
	return azNames, nil
}

// getExternalNeutronNetworksByAZ returns all external networks within the
// given availability zones. If azName is empty, return all external networks
// with no AZ.  If no network has an AZ, return all external networks.
func getExternalNeutronNetworksByAZ(ctx context.Context, e NetworkingBase, azNames set.Strings) ([]string, error) {
	neutronClient := e.neutron()
	// Find all external networks in availability zone.
	networks, err := neutronClient.ListNetworksV2(externalNetworkFilter())
	if err != nil {
		return nil, errors.Trace(err)
	}
	netIds := make([]string, 0)
	for _, network := range networks {
		for _, netAZ := range network.AvailabilityZones {
			if azNames.Contains(netAZ) {
				netIds = append(netIds, network.Id)
				break
			}
		}
		if azNames.IsEmpty() || len(network.AvailabilityZones) == 0 {
			logger.Debugf(ctx,
				"Adding %q to potential external networks for Floating IPs, no availability zones found", network.Name)
			netIds = append(netIds, network.Id)
		}
	}
	if len(netIds) == 0 {
		return nil, errors.NewNotFound(nil, "No External networks found to allocate a Floating IP")
	}
	return netIds, nil
}

// CreatePort creates a port for a given network id with a subnet ID.
func (n *NeutronNetworking) CreatePort(name, networkID string, subnetID network.Id) (*neutron.PortV2, error) {
	client := n.neutron()

	// To prevent name clashes to existing ports, generate a unique one from a
	// given name.
	portName := generateUniquePortName(name)

	port, err := client.CreatePortV2(neutron.PortV2{
		Name:        portName,
		Description: "Port created by juju for space aware networking",
		NetworkId:   networkID,
		FixedIPs: []neutron.PortFixedIPsV2{
			{
				SubnetID: subnetID.String(),
			},
		},
	})
	if err != nil {
		return nil, errors.Annotate(err, "unable to create port")
	}
	return port, nil
}

// DeletePortByID attempts to remove a port using the given port ID.
func (n *NeutronNetworking) DeletePortByID(portID string) error {
	client := n.neutron()

	return client.DeletePortV2(portID)
}

// FindNetworks returns a set of internal or external network names
// depending on the provided argument.
func (n *NeutronNetworking) FindNetworks(internal bool) (set.Strings, error) {
	var filter *neutron.Filter
	switch internal {
	case true:
		filter = internalNetworkFilter()
	case false:
		filter = externalNetworkFilter()
	}
	client := n.neutron()
	networks, err := client.ListNetworksV2(filter)
	if err != nil {
		return nil, errors.Trace(err)
	}
	names := set.NewStrings()
	for _, network := range networks {
		names.Add(network.Name)
	}
	return names, nil
}

// ResolveNetworks is part of the Networking interface.
func (n *NeutronNetworking) ResolveNetworks(name string, external bool) ([]neutron.NetworkV2, error) {
	if utils.IsValidUUIDString(name) {
		// NOTE: There is an OpenStack cloud, "whitestack", which has the
		// network used to create servers specified as an External network,
		// contrary to how all the other OpenStacks that we know of work.
		// Here we just retrieve the network, regardless of whether it is
		// internal or external
		net, err := n.neutron().GetNetworkV2(name)
		if err != nil {
			return nil, errors.Trace(err)
		}
		return []neutron.NetworkV2{*net}, nil
	}

	// Prior to OpenStack Rocky, empty strings in the neutron filters were
	// ignored. So name="" AND external-router=false returned a list of all
	// internal networks.
	// If the list was length one, the OpenStack provider would use it
	// without explicit user configuration.
	//
	// Rocky introduced an optional extension to neutron:
	// empty-string-filtering.
	// If configured, it means the empty string must be explicitly matched.
	//
	// To preserve the prior behavior, if the configured name is empty,
	// look for all networks matching the `external` argument for the
	// OpenStack project Juju is using.
	var filter *neutron.Filter
	switch {
	case name == "" && !external:
		filter = internalNetworkFilter()
	case name == "" && external:
		filter = externalNetworkFilter()
	default:
		filter = networkFilter(name, external)
	}

	networks, err := n.neutron().ListNetworksV2(filter)
	return networks, errors.Trace(err)
}

func generateUniquePortName(name string) string {
	unique := utils.RandomString(8, append(utils.LowerAlpha, utils.Digits...))
	return fmt.Sprintf("juju-%s-%s", name, unique)
}

func makeSubnetInfo(ctx context.Context, neutron NetworkingNeutron, subnet neutron.SubnetV2) (network.SubnetInfo, error) {
	_, _, err := net.ParseCIDR(subnet.Cidr)
	if err != nil {
		return network.SubnetInfo{}, errors.Annotatef(err, "skipping subnet %q, invalid CIDR", subnet.Cidr)
	}
	net, err := neutron.GetNetworkV2(subnet.NetworkId)
	if err != nil {
		return network.SubnetInfo{}, err
	}

	// TODO (hml) 2017-03-20:
	// With goose updates, VLANTag can be updated to be
	// network.segmentation_id, if network.network_type equals vlan
	info := network.SubnetInfo{
		CIDR:              subnet.Cidr,
		ProviderId:        network.Id(subnet.Id),
		ProviderNetworkId: network.Id(subnet.NetworkId),
		VLANTag:           0,
		AvailabilityZones: net.AvailabilityZones,
	}
	logger.Tracef(ctx, "found subnet with info %#v", info)
	return info, nil
}

// Subnets returns basic information about the specified subnets known
// by the provider for the specified instance or list of ids. subnetIds can be
// empty, in which case all known are returned.
func (n *NeutronNetworking) Subnets(subnetIds []network.Id) ([]network.SubnetInfo, error) {
	netIds := set.NewStrings()
	internalNets := n.ecfg().networks()

	for _, iNet := range internalNets {
		networks, err := n.ResolveNetworks(iNet, false)
		if err != nil {
			logger.Warningf(context.TODO(), "could not resolve internal network id for %q: %v", iNet, err)
			continue
		}
		for _, net := range networks {
			netIds.Add(net.Id)
		}
	}

	// Note, there are cases where we will detect an external
	// network without it being explicitly configured by the user.
	// When we get to a point where we start detecting spaces for users
	// on Openstack, we'll probably need to include better logic here.
	externalNet := n.ecfg().externalNetwork()
	if externalNet != "" {
		networks, err := n.ResolveNetworks(externalNet, true)
		if err != nil {
			logger.Warningf(context.TODO(), "could not resolve external network id for %q: %v", externalNet, err)
		} else {
			for _, net := range networks {
				netIds.Add(net.Id)
			}
		}
	}

	logger.Debugf(context.TODO(), "finding subnets in networks: %s", strings.Join(netIds.Values(), ", "))

	subIdSet := set.NewStrings()
	for _, subId := range subnetIds {
		subIdSet.Add(string(subId))
	}

	var results []network.SubnetInfo
	// TODO(jam): 2018-05-23 It is likely that ListSubnetsV2 could
	// take a Filter rather that doing the filtering client side.
	neutron := n.neutron()
	subnets, err := neutron.ListSubnetsV2()
	if err != nil {
		return nil, errors.Annotatef(err, "failed to retrieve subnets")
	}
	if len(subnetIds) == 0 {
		for _, subnet := range subnets {
			// TODO (manadart 2018-07-17): If there was an error resolving
			// an internal network ID, then no subnets will be discovered.
			// The user will get an error attempting to add machines to
			// this model and will have to update model config with a
			// network name; but this does not re-discover the subnets.
			// If subnets/spaces become important, we will have to address
			// this somehow.
			if !netIds.Contains(subnet.NetworkId) {
				logger.Tracef(context.TODO(), "ignoring subnet %q, part of network %q", subnet.Id, subnet.NetworkId)
				continue
			}
			subIdSet.Add(subnet.Id)
		}
	}
	for _, subnet := range subnets {
		if !subIdSet.Contains(subnet.Id) {
			logger.Tracef(context.TODO(), "subnet %q not in %v, skipping", subnet.Id, subnetIds)
			continue
		}
		subIdSet.Remove(subnet.Id)
		if info, err := makeSubnetInfo(context.TODO(), neutron, subnet); err == nil {
			// Error will already have been logged.
			results = append(results, info)
		}
	}
	if !subIdSet.IsEmpty() {
		return nil, errors.Errorf("failed to find the following subnet ids: %v", subIdSet.Values())
	}
	return results, nil
}

// NetworkInterfaces implements environs.NetworkingEnviron. It returns a
// slice where the i_th element contains the list of network interfaces
// for the i_th input instance ID.
//
// If none of the provided instance IDs exist, ErrNoInstances will be returned.
// If only a subset of the instance IDs exist, the result will contain a nil
// value for the missing instances and a ErrPartialInstances error will be
// returned.
func (n *NeutronNetworking) NetworkInterfaces(instanceIDs []instance.Id) ([]network.InterfaceInfos, error) {
	allSubnets, err := n.Subnets(nil)
	if err != nil {
		return nil, errors.Annotate(err, "listing subnets")
	}
	subnetIDToCIDR := make(map[string]string)
	for _, sub := range allSubnets {
		subnetIDToCIDR[sub.ProviderId.String()] = sub.CIDR
	}

	neutronClient := n.neutron()
	filter := projectIDFilter(n.client().TenantId())

	fips, err := neutronClient.ListFloatingIPsV2(filter)
	if err != nil {
		return nil, errors.Annotate(err, "listing floating IPs")
	}

	// Map private IP to public IP for every assigned FIP.
	fixedToFIP := make(map[string]string)
	for _, fip := range fips {
		if fip.FixedIP == "" {
			continue
		}
		fixedToFIP[fip.FixedIP] = fip.IP
	}

	allInstPorts, err := neutronClient.ListPortsV2(filter)
	if err != nil {
		return nil, errors.Annotate(err, "listing ports")
	}

	// Group ports by device ID.
	instIfaceMap := make(map[instance.Id][]neutron.PortV2)
	for _, instPort := range allInstPorts {
		devID := instance.Id(instPort.DeviceId)
		instIfaceMap[devID] = append(instIfaceMap[devID], instPort)
	}

	res := make([]network.InterfaceInfos, len(instanceIDs))
	var matchCount int

	for resIdx, instID := range instanceIDs {
		ifaceList, found := instIfaceMap[instID]
		if !found {
			continue
		}

		matchCount++
		res[resIdx] = mapInterfaceList(ifaceList, subnetIDToCIDR, fixedToFIP)
	}

	if matchCount == 0 {
		return nil, environs.ErrNoInstances
	} else if matchCount < len(instanceIDs) {
		return res, environs.ErrPartialInstances
	}

	return res, nil
}

func mapInterfaceList(
	in []neutron.PortV2, subnetIDToCIDR, fixedToFIP map[string]string,
) network.InterfaceInfos {
	var out = make(network.InterfaceInfos, len(in))

	for idx, port := range in {
		ni := network.InterfaceInfo{
			DeviceIndex: idx,
			ProviderId:  network.Id(port.Id),
			// NOTE(achilleasa): on microstack port.Name is always empty.
			InterfaceName: port.Name,
			Disabled:      port.Status != "ACTIVE",
			NoAutoStart:   false,
			InterfaceType: network.EthernetDevice,
			Origin:        network.OriginProvider,
			MACAddress:    network.NormalizeMACAddress(port.MACAddress),
		}

		for i, ipConf := range port.FixedIPs {
			providerAddr := network.NewMachineAddress(
				ipConf.IPAddress,
				network.WithConfigType(network.ConfigStatic),
				network.WithCIDR(subnetIDToCIDR[ipConf.SubnetID]),
			).AsProviderAddress()

			ni.Addresses = append(ni.Addresses, providerAddr)

			// If there is a FIP associated with this private IP,
			// add it as a public address.
			if fip := fixedToFIP[ipConf.IPAddress]; fip != "" {
				ni.ShadowAddresses = append(ni.ShadowAddresses, network.NewMachineAddress(
					fip,
					network.WithScope(network.ScopePublic),
					// TODO (manadart 2022-02-08): Other providers add these
					// addresses with the DHCP config type.
					// But this is not really correct.
					// We should consider another type for what are in effect
					// NATing arrangements, that better conveys the topology.
				).AsProviderAddress())
			}

			// If this is the first address, populate additional NIC details.
			if i == 0 {
				ni.ProviderSubnetId = network.Id(ipConf.SubnetID)
			}
		}

		out[idx] = ni
	}

	return out
}

func networkForSubnet(networks []neutron.NetworkV2, subnetID network.Id) (neutron.NetworkV2, error) {
	for _, neutronNet := range networks {
		for _, netSubnetID := range neutronNet.SubnetIds {
			if netSubnetID == subnetID.String() {
				return neutronNet, nil
			}
		}
	}

	return neutron.NetworkV2{}, errors.NotFoundf("network for subnet %q", subnetID)
}
