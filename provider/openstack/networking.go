// Copyright 2012-2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package openstack

import (
	"fmt"
	"net"
	"strings"

	"github.com/go-goose/goose/v4/neutron"
	"github.com/go-goose/goose/v4/nova"
	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"github.com/juju/utils/v3"

	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/network"
	corenetwork "github.com/juju/juju/core/network"
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

func processResolveNetworkIds(name string, networkIds []string) (string, error) {
	switch len(networkIds) {
	case 1:
		return networkIds[0], nil
	case 0:
		return "", errors.Errorf("no networks exist with label %q", name)
	}
	return "", errors.Errorf("multiple networks with label %q: %v", name, networkIds)
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
	extNetworkIds, err := n.getExternalNetworkIDsFromHostAddrs(detail.Addresses)
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
					logger.Debugf("found unassigned public ip: %v", fip.IP)
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
			logger.Debugf("allocated new public IP: %s", newfip.IP)
			return &newfip.IP, nil
		}
	}

	logger.Debugf("Unable to allocate a public IP")
	return nil, lastErr
}

// getExternalNetworkIDsFromHostAddrs returns a slice of external network IDs.
// If specified, the configured external network is returned. Otherwise search
// for an external network in the same availability zones as the provided
// server addresses.
func (n *NeutronNetworking) getExternalNetworkIDsFromHostAddrs(addrs map[string][]nova.IPAddress) ([]string, error) {
	extNetworkIds := make([]string, 0)
	neutronClient := n.neutron()
	externalNetwork := n.ecfg().externalNetwork()
	if externalNetwork != "" {
		// the config specified an external network, try it first.
		netId, err := resolveNeutronNetwork(neutronClient, externalNetwork, true)
		if err != nil {
			logger.Debugf("external network %s not found, search for one", externalNetwork)
		} else {
			logger.Debugf("using external network %q", externalNetwork)
			extNetworkIds = []string{netId}
		}
	}

	// We have an external network ID, no need to search.
	if len(extNetworkIds) > 0 {
		return extNetworkIds, nil
	}

	hostAddrAZs, err := n.findNetworkAZForHostAddrs(addrs)
	if err != nil {
		return nil, errors.NewNotFound(nil,
			fmt.Sprintf("could not find an external network in availability zone(s) %q", hostAddrAZs.SortedValues()))
	}

	// Create slice of network.Ids for external networks in the same AZ as
	// the instance's networks, to find an existing floating ip in, or allocate
	// a new floating ip from.
	extNetIds, _ := getExternalNeutronNetworksByAZ(n, hostAddrAZs)

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
func getExternalNeutronNetworksByAZ(e NetworkingBase, azNames set.Strings) ([]string, error) {
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
			logger.Debugf(
				"Adding %q to potential external networks for Floating IPs, no availability zones found", network.Name)
			netIds = append(netIds, network.Id)
		}
	}
	if len(netIds) == 0 {
		return nil, errors.NewNotFound(nil, "No External networks found to allocate a Floating IP")
	}
	return netIds, nil
}

// DefaultNetworks is part of the Networking interface.
func (n *NeutronNetworking) DefaultNetworks() ([]nova.ServerNetworks, error) {
	return []nova.ServerNetworks{}, nil
}

// CreatePort creates a port for a given network id with a subnet ID.
func (n *NeutronNetworking) CreatePort(name, networkID string, subnetID corenetwork.Id) (*neutron.PortV2, error) {
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

// ResolveNetwork is part of the Networking interface.
func (n *NeutronNetworking) ResolveNetwork(name string, external bool) (string, error) {
	return resolveNeutronNetwork(n.neutron(), name, external)
}

func generateUniquePortName(name string) string {
	unique := utils.RandomString(8, append(utils.LowerAlpha, utils.Digits...))
	return fmt.Sprintf("juju-%s-%s", name, unique)
}

func resolveNeutronNetwork(client NetworkingNeutron, name string, external bool) (string, error) {
	if utils.IsValidUUIDString(name) {
		// NOTE: There is an OpenStack cloud, whitestack, which has the network
		// used to create servers specified as an External network, contrary to
		// how all the other OpenStacks that we know of work. Juju can use this
		// OpenStack by setting the "network" config by UUID, which we do not
		// verify, nor check to ensure it's an internal network.
		// TODO hml 2021-08-03
		// Verify that the UUID is of a valid network, without type.
		return name, nil
	}
	// Mimic unintentional, now expected behavior. Prior to OpenStack Rocky,
	// empty strings in the neutron filters were ignored. name == "" AND
	// external-router=false, returned a list of all internal networks.
	// If the list was length one, the OpenStack provider would use it,
	// without explicit user configuration.
	//
	// Rocky introduced an optional extension to neutron: empty-string-filtering.
	// If configured, allows the empty string must be matched.  This reverses
	// the expected behavior.
	//
	// To keep the expected behavior, if the provided name is empty, look for
	// all networks matching external value for the configured OpenStack project
	// juju is using.
	var filter *neutron.Filter
	switch {
	case name == "" && !external:
		filter = internalNetworkFilter()
	case name == "" && external:
		filter = externalNetworkFilter()
	default:
		filter = networkFilter(name, external)
	}
	networks, err := client.ListNetworksV2(filter)
	if err != nil {
		return "", err
	}
	var networkIds []string
	for _, network := range networks {
		networkIds = append(networkIds, network.Id)
	}
	return processResolveNetworkIds(name, networkIds)
}

func makeSubnetInfo(neutron NetworkingNeutron, subnet neutron.SubnetV2) (corenetwork.SubnetInfo, error) {
	_, _, err := net.ParseCIDR(subnet.Cidr)
	if err != nil {
		return corenetwork.SubnetInfo{}, errors.Annotatef(err, "skipping subnet %q, invalid CIDR", subnet.Cidr)
	}
	net, err := neutron.GetNetworkV2(subnet.NetworkId)
	if err != nil {
		return corenetwork.SubnetInfo{}, err
	}

	// TODO (hml) 2017-03-20:
	// With goose updates, VLANTag can be updated to be
	// network.segmentation_id, if network.network_type equals vlan
	info := corenetwork.SubnetInfo{
		CIDR:              subnet.Cidr,
		ProviderId:        corenetwork.Id(subnet.Id),
		ProviderNetworkId: corenetwork.Id(subnet.NetworkId),
		VLANTag:           0,
		AvailabilityZones: net.AvailabilityZones,
	}
	logger.Tracef("found subnet with info %#v", info)
	return info, nil
}

// Subnets returns basic information about the specified subnets known
// by the provider for the specified instance or list of ids. subnetIds can be
// empty, in which case all known are returned.
func (n *NeutronNetworking) Subnets(instId instance.Id, subnetIds []corenetwork.Id) ([]corenetwork.SubnetInfo, error) {
	netIds := set.NewStrings()
	neutron := n.neutron()
	internalNet := n.ecfg().network()
	netId, err := resolveNeutronNetwork(neutron, internalNet, false)
	if err != nil {
		// Note: (jam 2018-05-23) We don't treat this as fatal because
		// we used to never pay attention to it anyway
		if internalNet == "" {
			logger.Warningf(noNetConfigMsg(err))
		} else {
			logger.Warningf("could not resolve internal network id for %q: %v", internalNet, err)
		}
	} else {
		netIds.Add(netId)
		// Note, there are cases where we will detect an external
		// network without it being explicitly configured by the user.
		// When we get to a point where we start detecting spaces for users
		// on Openstack, we'll probably need to include better logic here.
		externalNet := n.ecfg().externalNetwork()
		if externalNet != "" {
			netId, err := resolveNeutronNetwork(neutron, externalNet, true)
			if err != nil {
				logger.Warningf("could not resolve external network id for %q: %v", externalNet, err)
			} else {
				netIds.Add(netId)
			}
		}
	}
	logger.Debugf("finding subnets in networks: %s", strings.Join(netIds.Values(), ", "))

	subIdSet := set.NewStrings()
	for _, subId := range subnetIds {
		subIdSet.Add(string(subId))
	}

	var results []corenetwork.SubnetInfo
	if instId != instance.UnknownId {
		// TODO(hml): 2017-03-20
		// Implement Subnets() for case where instId is specified
		return nil, errors.NotSupportedf("neutron subnets with instance Id")
	} else {
		// TODO(jam): 2018-05-23 It is likely that ListSubnetsV2 could
		// take a Filter rather that doing the filtering client side.
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
					logger.Tracef("ignoring subnet %q, part of network %q", subnet.Id, subnet.NetworkId)
					continue
				}
				subIdSet.Add(subnet.Id)
			}
		}
		for _, subnet := range subnets {
			if !subIdSet.Contains(subnet.Id) {
				logger.Tracef("subnet %q not in %v, skipping", subnet.Id, subnetIds)
				continue
			}
			subIdSet.Remove(subnet.Id)
			if info, err := makeSubnetInfo(neutron, subnet); err == nil {
				// Error will already have been logged.
				results = append(results, info)
			}
		}
	}
	if !subIdSet.IsEmpty() {
		return nil, errors.Errorf("failed to find the following subnet ids: %v", subIdSet.Values())
	}
	return results, nil
}

// noNetConfigMsg is used to present resolution options when an error is
// encountered due to missing "network" configuration.
// Any error from attempting to resolve a network without network config set,
// is likely due to the resolution returning multiple internal networks.
func noNetConfigMsg(err error) string {
	return fmt.Sprintf(
		"%s\n\tTo resolve this error, set a value for \"network\" in model-config or model-defaults;"+
			"\n\tor supply it via --config when creating a new model",
		err.Error())
}

// NetworkInterfaces implements environs.NetworkingEnviron. It returns a
// slice where the i_th element contains the list of network interfaces
// for the i_th input instance ID.
//
// If none of the provided instance IDs exist, ErrNoInstances will be returned.
// If only a subset of the instance IDs exist, the result will contain a nil
// value for the missing instances and a ErrPartialInstances error will be
// returned.
func (n *NeutronNetworking) NetworkInterfaces(instanceIDs []instance.Id) ([]corenetwork.InterfaceInfos, error) {
	allSubnets, err := n.Subnets(instance.UnknownId, nil)
	if err != nil {
		return nil, errors.Annotate(err, "listing subnets")
	}
	subnetIDToCIDR := make(map[string]string)
	for _, sub := range allSubnets {
		subnetIDToCIDR[sub.ProviderId.String()] = sub.CIDR
	}

	// Unfortunately the client does not support filter by anything other
	// than tags, so we need to grab everything and apply the filter here.
	neutronClient := n.neutron()
	allInstPorts, err := neutronClient.ListPortsV2()
	if err != nil {
		return nil, errors.Annotate(err, "listing ports")
	}

	// Group ports by device ID.
	instIfaceMap := make(map[instance.Id][]neutron.PortV2)
	for _, instPort := range allInstPorts {
		devID := instance.Id(instPort.DeviceId)
		instIfaceMap[devID] = append(instIfaceMap[devID], instPort)
	}

	var (
		res        = make([]corenetwork.InterfaceInfos, len(instanceIDs))
		matchCount int
	)

	for resIdx, instID := range instanceIDs {
		ifaceList, found := instIfaceMap[instID]
		if !found {
			continue
		}

		matchCount++
		res[resIdx] = mapInterfaceList(ifaceList, subnetIDToCIDR)
	}

	if matchCount == 0 {
		return nil, environs.ErrNoInstances
	} else if matchCount < len(instanceIDs) {
		return res, environs.ErrPartialInstances
	}

	return res, nil
}

func mapInterfaceList(in []neutron.PortV2, subnetIDToCIDR map[string]string) network.InterfaceInfos {
	var out = make(corenetwork.InterfaceInfos, len(in))

	for idx, osIf := range in {
		ni := corenetwork.InterfaceInfo{
			DeviceIndex:       idx,
			ProviderId:        corenetwork.Id(osIf.Id),
			ProviderNetworkId: corenetwork.Id(osIf.NetworkId),
			InterfaceName:     osIf.Name, // NOTE(achilleasa): on microstack this is always empty.
			Disabled:          osIf.Status != "ACTIVE",
			NoAutoStart:       false,
			InterfaceType:     corenetwork.EthernetDevice,
			Origin:            corenetwork.OriginProvider,
			MACAddress:        corenetwork.NormalizeMACAddress(osIf.MACAddress),
		}

		for i, ipConf := range osIf.FixedIPs {
			addrOpts := []func(corenetwork.AddressMutator){
				// openstack assigns IPs from a configured pool.
				corenetwork.WithConfigType(corenetwork.ConfigStatic),
			}

			if subnetCIDR := subnetIDToCIDR[ipConf.SubnetID]; subnetCIDR != "" {
				addrOpts = append(addrOpts, corenetwork.WithCIDR(subnetCIDR))
			}

			providerAddr := corenetwork.NewMachineAddress(
				ipConf.IPAddress,
				addrOpts...,
			).AsProviderAddress()

			ni.Addresses = append(ni.Addresses, providerAddr)

			// If this is the first address, populate additional NIC details.
			if i == 0 {
				ni.ConfigType = corenetwork.ConfigStatic
				ni.ProviderSubnetId = corenetwork.Id(ipConf.SubnetID)
			}
		}

		out[idx] = ni
	}

	return out
}
