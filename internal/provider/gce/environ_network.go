// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package gce

import (
	"context"
	"fmt"
	"path"
	"strings"

	"cloud.google.com/go/compute/apiv1/computepb"
	"github.com/juju/collections/set"
	"github.com/juju/errors"

	"github.com/juju/juju/core/instance"
	corenetwork "github.com/juju/juju/core/network"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/instances"
	"github.com/juju/juju/internal/provider/gce/internal/google"
)

type subnetMap map[string]corenetwork.SubnetInfo

// Subnets implements environs.NetworkingEnviron.
func (e *environ) Subnets(
	ctx context.Context, subnetIds []corenetwork.Id,
) ([]corenetwork.SubnetInfo, error) {
	// In GCE all the subnets are in all AZs.
	zones, err := e.zoneNames(ctx)
	if err != nil {
		return nil, errors.Trace(err)
	}

	wantIDs := set.NewStrings()
	for _, id := range subnetIds {
		wantIDs.Add(id.String())
	}
	results, err := e.getSubnets(ctx, wantIDs, zones)
	if err != nil {
		return nil, errors.Trace(err)
	}

	missing := wantIDs.Difference(subnetIdsForSubnets(results))
	if missing.Size() != 0 {
		return nil, errors.NotFoundf("subnets %v", formatMissing(missing.SortedValues()))
	}

	return results, nil
}

func subnetIdsForSubnets(subnets []corenetwork.SubnetInfo) set.Strings {
	result := make([]string, len(subnets))
	for i, subnet := range subnets {
		result[i] = subnet.ProviderId.String()
	}
	return set.NewStrings(result...)
}

// getInstanceSubnet returns the vpc network and optionally a subnet to use when
// starting a new instance. The subnet will be empty if we are relying on the
// auto create subnet mode of the network.
func (env *environ) getInstanceSubnet(ctx context.Context) (string, *string, error) {
	vpcLink, autosubnets, err := env.getVpcInfo(ctx)
	if err != nil {
		return "", nil, errors.Trace(err)
	}
	if vpcLink == nil {
		// No VPC so just use default network.
		return fmt.Sprintf("%s%s", google.NetworkPathRoot, google.NetworkDefaultName), nil, nil
	}

	subnetworks, err := env.gce.NetworkSubnetworks(ctx, env.cloud.Region, *vpcLink)
	if err != nil {
		return "", nil, errors.Trace(err)
	}
	// Choose a subnet if there is one to choose from, or else rely on auto.
	if !autosubnets && len(subnetworks) == 0 {
		return "", nil, environs.ZoneIndependentError(
			errors.New("VPC does not auto create subnets and has no subnets"))
	}
	// Until placement is implemented, just use the first subnet.
	var subnetworkURL *string
	if len(subnetworks) > 0 {
		subnetworkURL = ptr(subnetworks[0].GetSelfLink())
	}
	return *vpcLink, subnetworkURL, nil
}

func (e *environ) zoneNames(ctx context.Context) ([]string, error) {
	zones, err := e.AvailabilityZones(ctx)
	if err != nil {
		return nil, errors.Trace(err)
	}
	names := make([]string, len(zones))
	for i, zone := range zones {
		names[i] = zone.Name()
	}
	return names, nil
}

type networkMap map[string]*computepb.Network

func (e *environ) networksByURL(ctx context.Context) (networkMap, error) {
	networks, err := e.gce.Networks(ctx)
	if err != nil {
		return nil, e.HandleCredentialError(ctx, err)
	}
	results := make(networkMap)
	for _, network := range networks {
		results[network.GetSelfLink()] = network
	}
	return results, nil
}

func (e *environ) getSubnets(
	ctx context.Context, subnetIds set.Strings, zones []string,
) ([]corenetwork.SubnetInfo, error) {
	// If subnets ids are specified, we'll load those, else fetch all subnets.
	// In the latter case, if a VPC is defined, use that to filter subnets,
	// else query for all networks; this is for compatibility when upgrading.
	// In either case the supplied subnet ids are matched against the set of
	// subnets for the relevant network(s).
	var (
		networks networkMap
		err      error
	)
	vpcID, haveVPC := e.vpcID()
	if haveVPC {
		network, err := e.gce.Network(ctx, vpcID)
		if err != nil {
			return nil, e.HandleCredentialError(ctx, err)
		}
		networks = networkMap{
			network.GetSelfLink(): network,
		}
	} else {
		networks, err = e.networksByURL(ctx)
		if err != nil {
			return nil, e.HandleCredentialError(ctx, err)
		}
	}

	// Get the subnet urls to query based on the vpc network.
	var urls []string
	subnetURLsByName := make(map[string]string)
	for _, network := range networks {
		for _, subnetURL := range network.Subnetworks {
			subnetName := path.Base(subnetURL)
			if subnetIds.IsEmpty() || subnetIds.Contains(subnetName) {
				subnetURLsByName[subnetName] = subnetURL
				urls = append(urls, subnetURL)
			}
		}
	}
	// Report on any missing networks.
	var notFoundSubnets []string
	for _, subnet := range subnetIds.Values() {
		if _, ok := subnetURLsByName[subnet]; !ok {
			notFoundSubnets = append(notFoundSubnets, subnet)
		}
	}
	if len(notFoundSubnets) > 0 {
		return nil, errors.NotFoundf("subnets %q", notFoundSubnets)
	}

	var allSubnets []*computepb.Subnetwork
	if len(urls) > 0 {
		if allSubnets, err = e.gce.Subnetworks(ctx, e.cloud.Region, urls...); err != nil {
			return nil, e.HandleCredentialError(ctx, err)
		}
	}

	var results []corenetwork.SubnetInfo
	for _, subnet := range allSubnets {
		results = append(results, makeSubnetInfo(
			corenetwork.Id(subnet.GetName()),
			corenetwork.Id(path.Base(subnet.GetNetwork())),
			subnet.GetIpCidrRange(),
			zones,
		))
	}
	// We have to include networks in 'LEGACY' mode that do not have subnetworks.
	for _, network := range networks {
		if network.GetIPv4Range() != "" &&
			(subnetIds.IsEmpty() || subnetIds.Contains(network.GetName())) {
			results = append(results, makeSubnetInfo(
				corenetwork.Id(network.GetName()),
				corenetwork.Id(network.GetName()),
				network.GetIPv4Range(),
				zones,
			))
		}
	}
	return results, nil
}

// NetworkInterfaces implements environs.NetworkingEnviron.
func (e *environ) NetworkInterfaces(ctx context.Context, ids []instance.Id) ([]corenetwork.InterfaceInfos, error) {
	if len(ids) == 0 {
		return nil, environs.ErrNoInstances
	}

	// Fetch instance information for the IDs we are interested in.
	insts, err := e.Instances(ctx, ids)
	partialInfo := err == environs.ErrPartialInstances
	if err != nil && err != environs.ErrPartialInstances {
		if errors.Cause(err) == environs.ErrNoInstances {
			return nil, err
		}
		return nil, errors.Trace(err)
	}

	vpcID, haveVPC := e.vpcID()
	if !haveVPC {
		vpcID = google.NetworkDefaultName
	}
	network, err := e.gce.Network(ctx, vpcID)
	if err != nil {
		return nil, errors.Trace(err)
	}

	// Extract the unique list of subnet URLs we are interested in for
	// all instances and fetch related subnet information.
	uniqueSubnetURLs, err := getUniqueSubnetURLs(ids, insts)
	if err != nil {
		return nil, errors.Trace(err)
	}

	// In GCE all the subnets are in all AZs.
	zones, err := e.zoneNames(ctx)
	if err != nil {
		return nil, errors.Trace(err)
	}
	subnets, err := e.subnetsByURL(ctx, network, uniqueSubnetURLs.SortedValues(), zones)
	if err != nil {
		return nil, errors.Trace(err)
	}

	infos := make([]corenetwork.InterfaceInfos, len(ids))
	for idx, inst := range insts {
		if inst == nil {
			continue // no instance with this ID known by provider
		}

		// Note: we have already verified that we can safely cast inst
		// to environInstance when we iterated the instance list to
		// obtain the unique subnet URLs
		for i, iface := range inst.(*environInstance).base.NetworkInterfaces {
			details, err := findNetworkDetails(iface, subnets, network)
			if err != nil {
				return nil, errors.Annotatef(err, "instance %q", ids[idx])
			}

			// Scan the access configs for public addresses
			var shadowAddrs corenetwork.ProviderAddresses
			for _, accessConf := range iface.AccessConfigs {
				// According to the gce docs only ONE_TO_ONE_NAT
				// is currently supported for external IPs
				if accessConf.GetType() != "ONE_TO_ONE_NAT" {
					continue
				}

				shadowAddrs = append(shadowAddrs,
					corenetwork.NewMachineAddress(accessConf.GetNatIP(), corenetwork.WithScope(corenetwork.ScopePublic)).AsProviderAddress(),
				)
			}

			infos[idx] = append(infos[idx], corenetwork.InterfaceInfo{
				DeviceIndex: i,
				// The network interface has no id in GCE so it's
				// identified by the machine's id + its name.
				ProviderId:    corenetwork.Id(fmt.Sprintf("%s/%s", ids[idx], iface.GetName())),
				InterfaceName: iface.GetName(),
				Addresses: corenetwork.ProviderAddresses{corenetwork.NewMachineAddress(
					iface.GetNetworkIP(),
					corenetwork.WithScope(corenetwork.ScopeCloudLocal),
					corenetwork.WithCIDR(details.cidr),
					corenetwork.WithConfigType(corenetwork.ConfigDHCP),
				).AsProviderAddress(
					corenetwork.WithProviderSubnetID(details.subnet),
				)},
				ShadowAddresses: shadowAddrs,
				InterfaceType:   corenetwork.EthernetDevice,
				Disabled:        false,
				NoAutoStart:     false,
				Origin:          corenetwork.OriginProvider,
			})
		}
	}

	if partialInfo {
		err = environs.ErrPartialInstances
	}
	return infos, err
}

func getUniqueSubnetURLs(ids []instance.Id, insts []instances.Instance) (set.Strings, error) {
	uniqueSet := set.NewStrings()

	for idx, inst := range insts {
		if inst == nil {
			continue // no instance with this ID known by provider
		}

		envInst, ok := inst.(*environInstance)
		if !ok { // This shouldn't happen.
			return nil, errors.Errorf("couldn't extract GCE instance for %q", ids[idx])
		}

		for _, iface := range envInst.base.NetworkInterfaces {
			if iface.GetSubnetwork() != "" {
				uniqueSet.Add(iface.GetSubnetwork())
			}
		}
	}

	return uniqueSet, nil
}

type networkDetails struct {
	cidr    string
	subnet  corenetwork.Id
	network corenetwork.Id
}

// findNetworkDetails looks up the network information we need to
// populate an InterfaceInfo - if the interface is on a legacy network
// we use information from the network because there'll be no subnet
// linked.
func findNetworkDetails(iface *computepb.NetworkInterface, subnets subnetMap, network *computepb.Network) (networkDetails, error) {
	var result networkDetails
	if iface.GetSubnetwork() == "" {
		result.cidr = network.GetIPv4Range()
		result.subnet = ""
		result.network = corenetwork.Id(network.GetName())
	} else {
		subnet, ok := subnets[iface.GetSubnetwork()]
		if !ok {
			return result, errors.NotFoundf("subnet %q", iface.Subnetwork)
		}
		result.cidr = subnet.CIDR
		result.subnet = subnet.ProviderId
		result.network = subnet.ProviderNetworkId
	}
	return result, nil
}

func (e *environ) subnetsByURL(ctx context.Context, network *computepb.Network, urls []string, zones []string) (subnetMap, error) {
	if len(urls) == 0 {
		return make(map[string]corenetwork.SubnetInfo), nil
	}
	urlSet := set.NewStrings(urls...)
	allSubnets, err := e.gce.Subnetworks(ctx, e.cloud.Region, urls...)
	if err != nil {
		return nil, e.HandleCredentialError(ctx, err)
	}
	results := make(map[string]corenetwork.SubnetInfo)
	for _, subnet := range allSubnets {
		urlSet.Remove(subnet.GetSelfLink())
		results[subnet.GetSelfLink()] = makeSubnetInfo(
			corenetwork.Id(subnet.GetName()),
			corenetwork.Id(network.GetName()),
			subnet.GetIpCidrRange(),
			zones,
		)
	}
	if urlSet.Size() > 0 {
		return nil, errors.NotFoundf("subnets %v", formatMissing(urlSet.Values()))
	}
	return results, nil
}

// SupportsSpaces implements environs.NetworkingEnviron.
func (e *environ) SupportsSpaces() (bool, error) {
	return false, nil
}

func copyStrings(items []string) []string {
	if items == nil {
		return nil
	}
	result := make([]string, len(items))
	copy(result, items)
	return result
}

func makeSubnetInfo(
	subnetId corenetwork.Id, networkId corenetwork.Id, cidr string, zones []string,
) corenetwork.SubnetInfo {
	return corenetwork.SubnetInfo{
		ProviderId:        subnetId,
		ProviderNetworkId: networkId,
		CIDR:              cidr,
		AvailabilityZones: copyStrings(zones),
		VLANTag:           0,
	}
}

func formatMissing(items []string) string {
	parts := make([]string, len(items))
	for i, item := range items {
		parts[i] = fmt.Sprintf("%q", item)
	}
	return "[" + strings.Join(parts, ", ") + "]"
}
