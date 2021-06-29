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
	"github.com/juju/utils/v2"

	"github.com/juju/juju/core/instance"
	corenetwork "github.com/juju/juju/core/network"
)

// Networking is an interface providing networking-related operations
// for an OpenStack Environ.
type Networking interface {
	// AllocatePublicIP allocates a public (floating) IP
	// to the specified instance.
	AllocatePublicIP(instance.Id) (*string, error)

	// DefaultNetworks returns the set of networks that should be
	// added by default to all new instances.
	DefaultNetworks() ([]nova.ServerNetworks, error)

	// ResolveNetwork takes either a network ID or label
	// with a string to specify whether the network is external
	// and returns the corresponding network ID.
	ResolveNetwork(string, bool) (string, error)

	// Subnets returns basic information about subnets known
	// by OpenStack for the environment.
	// Needed for Environ.Networking
	Subnets(instance.Id, []corenetwork.Id) ([]corenetwork.SubnetInfo, error)

	// CreatePort creates a port for a given network id with a subnet ID.
	CreatePort(string, string, corenetwork.Id) (*neutron.PortV2, error)

	// DeletePortByID attempts to remove a port using the given port ID.
	DeletePortByID(string) error

	// NetworkInterfaces requests information about the network
	// interfaces on the given list of instances.
	// Needed for Environ.Networking
	NetworkInterfaces(ids []instance.Id) ([]corenetwork.InterfaceInfos, error)

	// FindNetworks returns a set of internal or external network names
	// depending on the provided argument.
	FindNetworks(internal bool) (set.Strings, error)
}

// NetworkingDecorator is an interface that provides a means of overriding
// the default Networking implementation.
type NetworkingDecorator interface {
	// DecorateNetworking can be used to return a new Networking
	// implementation that overrides the provided, default Networking
	// implementation.
	DecorateNetworking(Networking) (Networking, error)
}

type networkingBase struct {
	env *Environ
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
	networkingBase
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
	return &NeutronNetworking{networkingBase: networkingBase{env: e}}
}

// AllocatePublicIP is part of the Networking interface.
func (n *NeutronNetworking) AllocatePublicIP(_ instance.Id) (*string, error) {
	// Look for an external network to use for the FIP.
	extNetworkIds, err := n.getExternalNetworkIDs()
	if err != nil {
		return nil, errors.Trace(err)
	}

	// Look for FIPs in same project as the credentials.
	// Admins have visibility into other projects.
	fips, err := n.env.neutron().ListFloatingIPsV2(projectIDFilter(n.env.client().TenantId()))
	if err != nil {
		return nil, errors.Trace(err)
	}

	// Is there an unused FloatingIP on an external network in the instance's availability zone?
	for _, fip := range fips {
		if fip.FixedIP == "" {
			// Not a perfect solution.  If an external network was specified in the
			// config, it'll be at the top of the extNetworkIds, but may be not used
			// if the available FIP isn't it in.  However the instance and the
			// FIP will be in the same availability zone.
			for _, extNetId := range extNetworkIds {
				if fip.FloatingNetworkId == extNetId {
					logger.Debugf("found unassigned public ip: %v", fip.IP)
					return &fip.IP, nil
				}
			}
		}
	}

	// allocate a new IP and use it
	var lastErr error
	for _, extNetId := range extNetworkIds {
		var newfip *neutron.FloatingIPV2
		newfip, lastErr = n.env.neutron().AllocateFloatingIPV2(extNetId)
		if lastErr == nil {
			logger.Debugf("allocated new public IP: %s", newfip.IP)
			return &newfip.IP, nil
		}
	}

	logger.Debugf("Unable to allocate a public IP")
	return nil, lastErr
}

// getExternalNetworkIDs returns a slice of external network IDs.
// If specified, the configured external network is returned.
// Otherwise search for an external network in the same
// availability zone as the private network.
func (n *NeutronNetworking) getExternalNetworkIDs() ([]string, error) {
	extNetworkIds := make([]string, 0)
	neutronClient := n.env.neutron()
	externalNetwork := n.env.ecfg().externalNetwork()
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

	// Create slice of network.Ids for external networks in the same AZ as
	// the instance's network, to find an existing floating ip in, or allocate
	// a new floating ip from.
	configNetwork := n.env.ecfg().network()
	netId, err := resolveNeutronNetwork(neutronClient, configNetwork, false)
	if err != nil {
		return nil, errors.Trace(err)
	}
	netDetails, err := neutronClient.GetNetworkV2(netId)
	if err != nil {
		return nil, errors.Trace(err)
	}

	availabilityZones := netDetails.AvailabilityZones
	if len(availabilityZones) == 0 {
		// No availability zones is valid, check for empty string
		// to ensure we still find the external network with no
		// AZ specified.  lp: 1891227
		availabilityZones = []string{""}
	}
	for _, az := range availabilityZones {
		extNetIds, _ := getExternalNeutronNetworksByAZ(n.env, az)
		if len(extNetIds) > 0 {
			extNetworkIds = append(extNetworkIds, extNetIds...)
		}
	}

	// We have an external network ID, no need for specific error message.
	if len(extNetworkIds) > 0 {
		return extNetworkIds, nil
	}

	var returnErr error
	if len(netDetails.AvailabilityZones) == 0 {
		returnErr = errors.NotFoundf("could not find an external network, network availability zones not in use")
	} else {
		returnErr = errors.NotFoundf(
			"could not find an external network in availability zone(s) %s", netDetails.AvailabilityZones)
	}
	return nil, returnErr
}

// getExternalNeutronNetworksByAZ returns all external networks within the
// given availability zone. If azName is empty, return all external networks
// with no AZ.
func getExternalNeutronNetworksByAZ(e *Environ, azName string) ([]string, error) {
	neutronClient := e.neutron()
	// Find all external networks in availability zone
	networks, err := neutronClient.ListNetworksV2(externalNetworkFilter())
	if err != nil {
		return nil, errors.Trace(err)
	}
	netIds := make([]string, 0)
	for _, network := range networks {
		for _, netAZ := range network.AvailabilityZones {
			if azName == netAZ {
				netIds = append(netIds, network.Id)
				break
			}
		}
		if azName == "" && len(network.AvailabilityZones) == 0 {
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
	client := n.env.neutron()

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
	client := n.env.neutron()

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
	client := n.env.neutron()
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
	return resolveNeutronNetwork(n.env.neutron(), name, external)
}

func generateUniquePortName(name string) string {
	unique := utils.RandomString(8, append(utils.LowerAlpha, utils.Digits...))
	return fmt.Sprintf("juju-%s-%s", name, unique)
}

func resolveNeutronNetwork(client *neutron.Client, name string, external bool) (string, error) {
	if utils.IsValidUUIDString(name) {
		return name, nil
	}
	// Mimic unintentional, now expected behavior. Prior to OpenStack Rocky,
	// empty strings in the neutron filters were ignored. name == "" AND
	// external-router=false, returned a list of all internal networks.  If
	// there the list was length one, the OpenStack provider would use it,
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

func makeSubnetInfo(neutron *neutron.Client, subnet neutron.SubnetV2) (corenetwork.SubnetInfo, error) {
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
	neutron := n.env.neutron()
	internalNet := n.env.ecfg().network()
	netId, err := resolveNeutronNetwork(neutron, internalNet, false)
	if err != nil {
		// Note: (jam 2018-05-23) We don't treat this as fatal because we used to never pay attention to it anyway
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
		externalNet := n.env.ecfg().externalNetwork()
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
// Any error from attempting to resolve a network without network
// config set, is likely due to the resolution returning multiple
// internal networks.
func noNetConfigMsg(err error) string {
	return fmt.Sprintf(
		"%s\n\tTo resolve this error, set a value for \"network\" in model-config or model-defaults;"+
			"\n\tor supply it via --config when creating a new model",
		err.Error())
}

func (n *NeutronNetworking) NetworkInterfaces(ids []instance.Id) ([]corenetwork.InterfaceInfos, error) {
	return nil, errors.NotSupportedf("neutron network interfaces")
}
