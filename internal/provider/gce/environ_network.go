// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package gce

import (
	"fmt"
	"math/rand"
	"path"
	"strings"

	"cloud.google.com/go/compute/apiv1/computepb"
	"github.com/juju/collections/set"
	"github.com/juju/errors"

	"github.com/juju/juju/core/instance"
	corenetwork "github.com/juju/juju/core/network"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/context"
	"github.com/juju/juju/environs/instances"
	"github.com/juju/juju/internal/provider/common"
	"github.com/juju/juju/internal/provider/gce/internal/google"
)

const (
	// ErrNoSubnets indicates there are no subnets to use when creating an instance.
	ErrNoSubnets = errors.ConstError("VPC does not auto create subnets and has no subnets")
	// ErrAutoSubnetsInvalid indicates auto subnets cannot be used with placement or spaces.
	ErrAutoSubnetsInvalid = errors.ConstError("cannot use auto subnets")
)

type subnetMap map[string]corenetwork.SubnetInfo

// Subnets implements environs.NetworkingEnviron.
func (e *environ) Subnets(
	ctx context.ProviderCallContext, inst instance.Id, subnetIds []corenetwork.Id,
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
	var results []corenetwork.SubnetInfo
	if inst == instance.UnknownId {
		results, err = e.getSubnets(ctx, wantIDs, zones)
	} else {
		results, err = e.getInstanceSubnets(ctx, inst, wantIDs, zones)
	}
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

// placementSubnet finds a subnet matching the supplied spec.
// The match is done on subnet name or ipv4 cidr range.
func placementSubnet(spec string, subnets []*computepb.Subnetwork) *computepb.Subnetwork {
	for _, s := range subnets {
		if s.GetName() == spec {
			return s
		}
		if s.GetIpCidrRange() == spec {
			return s
		}
	}
	return nil
}

func (env *environ) subnetsForInstance(ctx context.ProviderCallContext, args environs.StartInstanceParams) (*string, []*computepb.Subnetwork, error) {
	vpcLink, autosubnets, err := env.getVpcInfo(ctx)
	if err != nil {
		return nil, nil, errors.Trace(err)
	}
	if vpcLink == nil {
		// No VPC so just use default network.
		vpcLink = ptr(fmt.Sprintf("%s%s", google.NetworkPathRoot, google.NetworkDefaultName))
	}

	allSubnets, err := env.gce.NetworkSubnetworks(ctx, env.cloud.Region, *vpcLink)
	if err != nil {
		return nil, nil, google.HandleCredentialError(errors.Trace(err), ctx)
	}

	instPlacement, err := env.parsePlacement(args.Placement)
	if err != nil {
		return nil, nil, environs.ZoneIndependentError(err)
	}

	if autosubnets && len(allSubnets) == 0 {
		if spec := instPlacement.subnetSpec; spec != "" {
			return nil, nil, environs.ZoneIndependentError(
				errors.Wrap(ErrAutoSubnetsInvalid,
					errors.Errorf("cannot use placement subnet %q when using autosubnets", spec)),
			)
		}
		if args.Constraints.HasSpaces() {
			return nil, nil, environs.ZoneIndependentError(
				errors.Wrap(ErrAutoSubnetsInvalid,
					errors.New("cannot use space constraints when using autosubnets")),
			)
		}
		// Use the project's default network.
		return vpcLink, nil, nil
	}

	// Choose a subnet if there is one to choose from, or else rely on auto.
	if len(allSubnets) == 0 {
		return nil, nil, environs.ZoneIndependentError(ErrNoSubnets)
	}

	var allSubnetIDs = make([]corenetwork.Id, len(allSubnets))
	for i, subnet := range allSubnets {
		allSubnetIDs[i] = corenetwork.Id(subnet.GetName())
	}
	logger.Debugf("got subnets for vpc %q in region %q: %s", *vpcLink, env.cloud.Region, allSubnetIDs)

	constraints := args.Constraints

	// We'll collect all the possible subnets and pick one
	// based on constraints and placement.
	var (
		possibleSubnets   [][]corenetwork.Id
		placementSubnetID corenetwork.Id
	)

	if constraints.HasSpaces() {
		validSubnets, err := common.GetValidSubnetZoneMap(args)
		if err != nil {
			return nil, nil, environs.ZoneIndependentError(err)
		}
		var subnetIDs []corenetwork.Id
		for subnetID := range validSubnets {
			subnetIDs = append(subnetIDs, subnetID)
		}
		possibleSubnets = [][]corenetwork.Id{subnetIDs}
	} else {
		// Use placement if specified.
		if instPlacement.subnetSpec != "" {
			subnet := placementSubnet(instPlacement.subnetSpec, allSubnets)
			if subnet == nil {
				return nil, nil, environs.ZoneIndependentError(
					errors.NotFoundf("placement subnet %q", placementSubnetID))
			}
			return vpcLink, []*computepb.Subnetwork{subnet}, nil
		}
		possibleSubnets = [][]corenetwork.Id{allSubnetIDs}
	}

	// For each list of subnet IDs that satisfy space constraints,
	// choose a single one at random.
	var subnetIDForZone []corenetwork.Id
	for _, zoneSubnetIDs := range possibleSubnets {
		// Use placement to select a single subnet if needed.
		var subnetIDs []corenetwork.Id
		if instPlacement.subnetSpec == "" {
			// No placement so we can select from all subnets.
			subnetIDs = zoneSubnetIDs
		} else {
			if subnet := placementSubnet(instPlacement.subnetSpec, allSubnets); subnet != nil {
				// Record the placement subnet id so it can be added first sown below.
				placementSubnetID = corenetwork.Id(subnet.GetName())
				subnetIDs = []corenetwork.Id{corenetwork.Id(subnet.GetName())}
			}
		}
		if len(subnetIDs) == 1 {
			subnetIDForZone = append(subnetIDForZone, subnetIDs[0])
			continue
		} else if len(subnetIDs) > 0 {
			// Do we want to prefer dual stack subnets?
			subnetIDForZone = append(subnetIDForZone, subnetIDs[rand.Intn(len(subnetIDs))])
		}
	}
	if len(subnetIDForZone) == 0 {
		return nil, nil, environs.ZoneIndependentError(
			errors.NotFoundf("subnet for constraint %q", constraints.String()))
	}

	var subnetIds []corenetwork.Id
	// Put any placement subnet first in the list
	// so it ia allocated to the primary NIC.
	if placementSubnetID != "" {
		subnetIds = append(subnetIds, placementSubnetID)
	}
	for _, id := range subnetIDForZone {
		if id != placementSubnetID {
			subnetIds = append(subnetIds, id)
		}
	}

	// Collate the results.
	subnetsByID := make(map[corenetwork.Id]*computepb.Subnetwork)
	for _, subnet := range allSubnets {
		subnetsByID[corenetwork.Id(subnet.GetName())] = subnet
	}
	result := make([]*computepb.Subnetwork, len(subnetIds))
	for i, subnetId := range subnetIds {
		subnet, ok := subnetsByID[subnetId]
		if !ok {
			return nil, nil, environs.ZoneIndependentError(
				errors.NotFoundf("subnet %q not found", subnetId))
		}
		result[i] = subnet
	}
	return vpcLink, result, nil
}

func (e *environ) zoneNames(ctx context.ProviderCallContext) ([]string, error) {
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

func (e *environ) networksByURL(ctx context.ProviderCallContext) (networkMap, error) {
	networks, err := e.gce.Networks(ctx)
	if err != nil {
		return nil, google.HandleCredentialError(errors.Trace(err), ctx)
	}
	results := make(networkMap)
	for _, network := range networks {
		results[network.GetSelfLink()] = network
	}
	return results, nil
}

func (e *environ) getSubnets(
	ctx context.ProviderCallContext, subnetIds set.Strings, zones []string,
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
			return nil, google.HandleCredentialError(errors.Trace(err), ctx)
		}
		networks = networkMap{
			network.GetSelfLink(): network,
		}
	} else {
		networks, err = e.networksByURL(ctx)
		if err != nil {
			return nil, google.HandleCredentialError(errors.Trace(err), ctx)
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
			return nil, google.HandleCredentialError(errors.Trace(err), ctx)
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

func (e *environ) getInstanceSubnets(
	ctx context.ProviderCallContext, inst instance.Id, subnetIds set.Strings, zones []string,
) ([]corenetwork.SubnetInfo, error) {
	ifLists, err := e.NetworkInterfaces(ctx, []instance.Id{inst})
	if err != nil {
		return nil, errors.Trace(err)
	}
	ifaces := ifLists[0]

	var results []corenetwork.SubnetInfo
	for _, iface := range ifaces {
		if len(subnetIds) == 0 || subnetIds.Contains(string(iface.ProviderSubnetId)) {
			results = append(results, makeSubnetInfo(
				iface.ProviderSubnetId,
				iface.ProviderNetworkId,
				iface.PrimaryAddress().CIDR,
				zones,
			))
		}
	}
	return results, nil
}

// NetworkInterfaces implements environs.NetworkingEnviron.
func (e *environ) NetworkInterfaces(ctx context.ProviderCallContext, ids []instance.Id) ([]corenetwork.InterfaceInfos, error) {
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
				ProviderId:        corenetwork.Id(fmt.Sprintf("%s/%s", ids[idx], iface.GetName())),
				ProviderSubnetId:  details.subnet,
				ProviderNetworkId: details.network,
				AvailabilityZones: copyStrings(zones),
				InterfaceName:     iface.GetName(),
				Addresses: corenetwork.ProviderAddresses{corenetwork.NewMachineAddress(
					iface.GetNetworkIP(),
					corenetwork.WithScope(corenetwork.ScopeCloudLocal),
					corenetwork.WithCIDR(details.cidr),
					corenetwork.WithConfigType(corenetwork.ConfigDHCP),
				).AsProviderAddress()},
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

func (e *environ) subnetsByURL(ctx context.ProviderCallContext, network *computepb.Network, urls []string, zones []string) (subnetMap, error) {
	if len(urls) == 0 {
		return make(map[string]corenetwork.SubnetInfo), nil
	}
	urlSet := set.NewStrings(urls...)
	allSubnets, err := e.gce.Subnetworks(ctx, e.cloud.Region, urls...)
	if err != nil {
		return nil, google.HandleCredentialError(errors.Trace(err), ctx)
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
func (e *environ) SupportsSpaces(ctx context.ProviderCallContext) (bool, error) {
	return true, nil
}

// AreSpacesRoutable implements environs.NetworkingEnviron.
func (*environ) AreSpacesRoutable(ctx context.ProviderCallContext, space1, space2 *environs.ProviderSpaceInfo) (bool, error) {
	return false, nil
}

// SuperSubnets implements environs.SuperSubnets
func (e *environ) SuperSubnets(ctx context.ProviderCallContext) ([]string, error) {
	vpcLink, _, err := e.getVpcInfo(ctx)
	if err != nil {
		return nil, google.HandleCredentialError(errors.Trace(err), ctx)
	}
	subnets, err := e.gce.Subnetworks(ctx, e.cloud.Region)
	if err != nil {
		return nil, err
	}
	var cidrs []string
	for _, subnet := range subnets {
		if vpcLink == nil || subnet.GetNetwork() == *vpcLink {
			cidrs = append(cidrs, subnet.GetIpCidrRange())
		}
	}
	return cidrs, nil
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
