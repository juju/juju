// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package network

import (
	"math/big"
	"net"
	"sort"
	"strings"

	"github.com/juju/collections/set"
	"github.com/juju/errors"

	"github.com/juju/juju/v2/core/life"
)

// FanCIDRs describes the subnets relevant to a fan network.
type FanCIDRs struct {
	// FanLocalUnderlay is the CIDR of the local underlying fan network.
	// It allows easy identification of the device the fan is running on.
	FanLocalUnderlay string

	// FanOverlay is the CIDR of the complete fan setup.
	FanOverlay string
}

func newFanCIDRs(overlay, underlay string) *FanCIDRs {
	return &FanCIDRs{
		FanLocalUnderlay: underlay,
		FanOverlay:       overlay,
	}
}

// SubnetInfo is a source-agnostic representation of a subnet.
// It may originate from state, or from a provider.
type SubnetInfo struct {
	// ID is the unique ID of the subnet.
	ID Id

	// CIDR of the network, in 123.45.67.89/24 format.
	CIDR string

	// Memoized value for the parsed network for the above CIDR.
	parsedCIDRNetwork *net.IPNet

	// ProviderId is a provider-specific subnet ID.
	ProviderId Id

	// ProviderSpaceId holds the provider ID of the space associated
	// with this subnet. Can be empty if not supported.
	ProviderSpaceId Id

	// ProviderNetworkId holds the provider ID of the network
	// containing this subnet, for example VPC id for EC2.
	ProviderNetworkId Id

	// VLANTag needs to be between 1 and 4094 for VLANs and 0 for
	// normal networks. It's defined by IEEE 802.1Q standard, and used
	// to define a VLAN network. For more information, see:
	// http://en.wikipedia.org/wiki/IEEE_802.1Q.
	VLANTag int

	// AvailabilityZones describes which availability zones this
	// subnet is in. It can be empty if the provider does not support
	// availability zones.
	AvailabilityZones []string

	// SpaceID is the id of the space the subnet is associated with.
	// Default value should be AlphaSpaceId. It can be empty if
	// the subnet is returned from an networkingEnviron. SpaceID is
	// preferred over SpaceName in state and non networkingEnviron use.
	SpaceID string

	// SpaceName is the name of the space the subnet is associated with.
	// An empty string indicates it is part of the AlphaSpaceName OR
	// if the SpaceID is set. Should primarily be used in an networkingEnviron.
	SpaceName string

	// FanInfo describes the fan networking setup for the subnet.
	// It may be empty if this is not a fan subnet,
	// or if this subnet information comes from a provider.
	FanInfo *FanCIDRs

	// IsPublic describes whether a subnet is public or not.
	IsPublic bool

	// Life represents the current life-cycle status of the subnets.
	Life life.Value
}

// SetFan sets the fan networking information for the subnet.
func (s *SubnetInfo) SetFan(underlay, overlay string) {
	s.FanInfo = newFanCIDRs(overlay, underlay)
}

// FanLocalUnderlay returns the fan underlay CIDR if known.
func (s *SubnetInfo) FanLocalUnderlay() string {
	if s.FanInfo == nil {
		return ""
	}
	return s.FanInfo.FanLocalUnderlay
}

// FanOverlay returns the fan overlay CIDR if known.
func (s *SubnetInfo) FanOverlay() string {
	if s.FanInfo == nil {
		return ""
	}
	return s.FanInfo.FanOverlay
}

// Validate validates the subnet, checking the CIDR, and VLANTag, if present.
func (s *SubnetInfo) Validate() error {
	if s.CIDR == "" {
		return errors.Errorf("missing CIDR")
	} else if _, err := s.ParsedCIDRNetwork(); err != nil {
		return errors.Trace(err)
	}

	if s.VLANTag < 0 || s.VLANTag > 4094 {
		return errors.Errorf("invalid VLAN tag %d: must be between 0 and 4094", s.VLANTag)
	}

	return nil
}

// ParsedCIDRNetwork returns the network represented by the CIDR field.
func (s *SubnetInfo) ParsedCIDRNetwork() (*net.IPNet, error) {
	// Memoize the CIDR the first time this method is called or if the
	// CIDR field has changed.
	if s.parsedCIDRNetwork == nil || s.parsedCIDRNetwork.String() != s.CIDR {
		_, ipNet, err := net.ParseCIDR(s.CIDR)
		if err != nil {
			return nil, err
		}

		s.parsedCIDRNetwork = ipNet
	}
	return s.parsedCIDRNetwork, nil
}

// SubnetInfos is a collection of subnets.
type SubnetInfos []SubnetInfo

// SpaceIDs returns the set of space IDs that these subnets are in.
func (s SubnetInfos) SpaceIDs() set.Strings {
	spaceIDs := set.NewStrings()
	for _, sub := range s {
		spaceIDs.Add(sub.SpaceID)
	}
	return spaceIDs
}

// GetByUnderlayCIDR returns any subnets in this collection that are fan
// overlays for the input CIDR.
// An error is returned if the input is not a valid CIDR.
// TODO (manadart 2020-04-15): Consider storing subnet IDs in FanInfo,
// so we can ensure uniqueness in multi-network deployments.
func (s SubnetInfos) GetByUnderlayCIDR(cidr string) (SubnetInfos, error) {
	if !IsValidCIDR(cidr) {
		return nil, errors.NotValidf("CIDR %q", cidr)
	}

	var overlays SubnetInfos
	for _, sub := range s {
		if sub.FanLocalUnderlay() == cidr {
			overlays = append(overlays, sub)
		}
	}
	return overlays, nil
}

// ContainsID returns true if the collection contains a
// space with the given ID.
func (s SubnetInfos) ContainsID(id Id) bool {
	return s.GetByID(id) != nil
}

// GetByID returns a reference to the subnet with the input ID if one is found.
func (s SubnetInfos) GetByID(id Id) *SubnetInfo {
	for _, sub := range s {
		if sub.ID == id {
			return &sub
		}
	}
	return nil
}

// GetByCIDR returns all subnets in the collection
// with a CIDR matching the input.
func (s SubnetInfos) GetByCIDR(cidr string) (SubnetInfos, error) {
	if !IsValidCIDR(cidr) {
		return nil, errors.NotValidf("CIDR %q", cidr)
	}

	var matching SubnetInfos
	for _, sub := range s {
		if sub.CIDR == cidr {
			matching = append(matching, sub)
		}
	}

	if len(matching) != 0 {
		return matching, nil
	}

	// Some providers (e.g. equinix) carve subnets into smaller CIDRs and
	// assign addresses from the carved subnets to the machines. If we were
	// not able to find a direct CIDR match fallback to a CIDR is sub-CIDR
	// of check.
	firstIP, lastIP, err := IPRangeForCIDR(cidr)
	if err != nil {
		return nil, errors.Annotatef(err, "unable to extract first and last IP addresses from CIDR %q", cidr)
	}

	for _, sub := range s {
		subNet, err := sub.ParsedCIDRNetwork()
		if err != nil { // this should not happen; but let's be paranoid.
			logger.Warningf("unable to parse CIDR %q for subnet %q", sub.CIDR, sub.ID)
			continue
		}

		if subNet.Contains(firstIP) && subNet.Contains(lastIP) {
			matching = append(matching, sub)
		}
	}

	return matching, nil
}

// GetByAddress returns subnets that based on IP range,
// include the input IP address.
func (s SubnetInfos) GetByAddress(addr string) (SubnetInfos, error) {
	ip := net.ParseIP(addr)
	if ip == nil {
		return nil, errors.NotValidf("%q as IP address", addr)
	}

	var subs SubnetInfos
	for _, sub := range s {
		ipNet, err := sub.ParsedCIDRNetwork()
		if err != nil {
			return nil, errors.Trace(err)
		}
		if ipNet.Contains(ip) {
			subs = append(subs, sub)
		}
	}
	return subs, nil
}

// GetBySpaceID returns all subnets with the input space ID,
// including those inferred by being overlays of subnets in the space.
func (s SubnetInfos) GetBySpaceID(spaceID string) (SubnetInfos, error) {
	var subsInSpace SubnetInfos
	for _, sub := range s {
		if sub.SpaceID == spaceID {
			subsInSpace = append(subsInSpace, sub)
		}
	}

	var spaceOverlays SubnetInfos
	for _, sub := range subsInSpace {
		// If we picked up an overlay because the space was already set,
		// don't try to find subnets for which it is an underlay.
		if sub.FanInfo != nil {
			continue
		}

		// TODO (manadart 2020-05-13): See comment for GetByUnderlayCIDR.
		// This will only be correct for unique CIDRs.
		overlays, err := s.GetByUnderlayCIDR(sub.CIDR)
		if err != nil {
			return nil, errors.Trace(err)
		}

		// Don't include overlays that already have a space ID.
		// They will have been retrieved as subsInSpace.
		for _, overlay := range overlays {
			if overlay.SpaceID == "" {
				overlay.SpaceID = spaceID
				spaceOverlays = append(spaceOverlays, overlay)
			}
		}
	}

	return append(subsInSpace, spaceOverlays...), nil
}

// AllSubnetInfos implements SubnetLookup
// by returning all of the subnets.
func (s SubnetInfos) AllSubnetInfos() (SubnetInfos, error) {
	return s, nil
}

// EqualTo returns true if this slice of SubnetInfo is equal to the input.
func (s SubnetInfos) EqualTo(other SubnetInfos) bool {
	if len(s) != len(other) {
		return false
	}

	SortSubnetInfos(s)
	SortSubnetInfos(other)
	for i := 0; i < len(s); i++ {
		if s[i].ID != other[i].ID {
			return false
		}
	}

	return true
}

func (s SubnetInfos) Len() int      { return len(s) }
func (s SubnetInfos) Swap(i, j int) { s[i], s[j] = s[j], s[i] }
func (s SubnetInfos) Less(i, j int) bool {
	return s[i].ID < s[j].ID
}

// SortSubnetInfos sorts subnets by ID.
func SortSubnetInfos(s SubnetInfos) {
	sort.Sort(s)
}

// IsValidCIDR returns whether cidr is a valid subnet CIDR.
func IsValidCIDR(cidr string) bool {
	_, ipNet, err := net.ParseCIDR(cidr)
	if err == nil && ipNet.String() == cidr {
		return true
	}
	return false
}

// FindSubnetIDsForAvailabilityZone returns a series of subnet IDs from a series
// of zones, if zones match the zoneName.
//
// Returns an error if no matching subnets match the zoneName.
func FindSubnetIDsForAvailabilityZone(zoneName string, subnetsToZones map[Id][]string) ([]Id, error) {
	matchingSubnetIDs := set.NewStrings()
	for subnetID, zones := range subnetsToZones {
		zonesSet := set.NewStrings(zones...)
		if zonesSet.Size() == 0 && zoneName == "" || zonesSet.Contains(zoneName) {
			matchingSubnetIDs.Add(string(subnetID))
		}
	}

	if matchingSubnetIDs.IsEmpty() {
		return nil, errors.NotFoundf("subnets in AZ %q", zoneName)
	}

	sorted := make([]Id, matchingSubnetIDs.Size())
	for k, v := range matchingSubnetIDs.SortedValues() {
		sorted[k] = Id(v)
	}

	return sorted, nil
}

// InFan describes a network fan type.
const InFan = "INFAN"

// FilterInFanNetwork filters out any fan networks.
func FilterInFanNetwork(networks []Id) []Id {
	var result []Id
	for _, network := range networks {
		if !IsInFanNetwork(network) {
			result = append(result, network)
		}
	}
	return result
}

func IsInFanNetwork(network Id) bool {
	return strings.Contains(network.String(), InFan)
}

// IPRangeForCIDR returns the first and last addresses that correspond to the
// provided CIDR. The first address will always be the network address. The
// returned range also includes the broadcast address. For example, a CIDR of
// 10.0.0.0/24 yields: [10.0.0.0, 10.0.0.255].
func IPRangeForCIDR(cidr string) (net.IP, net.IP, error) {
	_, ipNet, err := net.ParseCIDR(cidr)
	if err != nil {
		return net.IP{}, net.IP{}, errors.Trace(err)
	}
	ones, numBits := ipNet.Mask.Size()

	// Special case: CIDR specifies a single address (i.e. a /32 or /128
	// for IPV4 and IPV6 CIDRs accordingly).
	if ones == numBits {
		firstIP := ipNet.IP
		lastIP := make(net.IP, len(firstIP))
		copy(lastIP, firstIP)
		return firstIP, lastIP, nil
	}

	// Calculate number of hosts in network (2^hostBits - 1) or the
	// equivalent (1 << hostBits) - 1.
	hostCount := big.NewInt(1)
	hostCount = hostCount.Lsh(hostCount, uint(numBits-ones))
	hostCount = hostCount.Sub(hostCount, big.NewInt(1))

	// Calculate last IP in range.
	lastIPNum := big.NewInt(0).SetBytes([]byte(ipNet.IP))
	lastIPNum = lastIPNum.Add(lastIPNum, hostCount)

	// Convert last IP into bytes. Since BigInt strips off leading zeroes
	// we need to prepend them again before casting back to net.IP.
	lastIPBytes := lastIPNum.Bytes()
	lastIPBytes = append(make([]byte, len(ipNet.IP)-len(lastIPBytes)), lastIPBytes...)

	return ipNet.IP, net.IP(lastIPBytes), nil
}
