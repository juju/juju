// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package network

import (
	"context"
	"fmt"
	"net"
	"regexp"
	"strings"

	"github.com/juju/collections/set"
	"github.com/juju/errors"
)

const (
	// AlphaSpaceId is the ID of the alpha network space.
	// Application endpoints are bound to this space by default
	// if no explicit binding is specified.
	AlphaSpaceId = "0"

	// AlphaSpaceName is the name of the alpha network space.
	AlphaSpaceName = "alpha"
)

// SpaceLookup describes the ability to get a complete
// network topology, as understood by Juju.
type SpaceLookup interface {
	GetAllSpaces(ctx context.Context) (SpaceInfos, error)
}

// SubnetLookup describes retrieving all subnets within a known set of spaces.
type SubnetLookup interface {
	AllSubnetInfos() (SubnetInfos, error)
}

// SpaceName is the name of a network space.
type SpaceName string

// SpaceInfo defines a network space.
type SpaceInfo struct {
	// ID is the unique identifier for the space.
	// TODO (manadart 2020-04-10): This should be a typed ID.
	ID string

	// Name is the name of the space.
	// It is used by operators for identifying a space and should be unique.
	Name SpaceName

	// ProviderId is the provider's unique identifier for the space,
	// such as used by MAAS.
	ProviderId Id

	// Subnets are the subnets that have been grouped into this network space.
	Subnets SubnetInfos
}

// SpaceInfos is a collection of spaces.
type SpaceInfos []SpaceInfo

// AllSpaceInfos satisfies the SpaceLookup interface.
// It is useful for passing to conversions where we already have the spaces
// materialised and don't need to pull them from the DB again.
func (s SpaceInfos) AllSpaceInfos() (SpaceInfos, error) {
	return s, nil
}

// AllSubnetInfos returns all subnets contained in this collection of spaces.
// Since a subnet can only be in one space, we can simply accrue them all
// with the need for duplicate checking.
// As with AllSpaceInfos, it implements an interface that can be used to
// indirect state.
func (s SpaceInfos) AllSubnetInfos() (SubnetInfos, error) {
	subs := make(SubnetInfos, 0)
	for _, space := range s {
		for _, sub := range space.Subnets {
			subs = append(subs, sub)
		}
	}
	return subs, nil
}

// FanOverlaysFor returns any subnets in this network topology that are
// fan overlays for the input subnet IDs.
func (s SpaceInfos) FanOverlaysFor(subnetIDs IDSet) (SubnetInfos, error) {
	if len(subnetIDs) == 0 {
		return nil, nil
	}

	var located int
	var allOverlays SubnetInfos

	for _, space := range s {
		for _, sub := range space.Subnets {
			if subnetIDs.Contains(sub.ID) {
				overlays, err := space.Subnets.GetByUnderlayCIDR(sub.CIDR)
				if err != nil {
					return nil, errors.Trace(err)
				}
				allOverlays = append(allOverlays, overlays...)

				// If we have tested all of the inputs, we can exit early.
				located++
				if located >= len(subnetIDs) {
					return allOverlays, nil
				}
			}
		}
	}

	return allOverlays, nil
}

// MoveSubnets returns a new topology representing
// the movement of subnets to a new network space.
func (s SpaceInfos) MoveSubnets(subnetIDs IDSet, spaceName string) (SpaceInfos, error) {
	newSpace := s.GetByName(spaceName)
	if newSpace == nil {
		return nil, errors.NotFoundf("space with name %q", spaceName)
	}

	// We return a copy, not mutating the original.
	newSpaces := make(SpaceInfos, len(s))
	var movers SubnetInfos
	found := MakeIDSet()

	// First accrue the moving subnets and remove them from their old spaces.
	for i, space := range s {
		newSpaces[i] = space
		newSpaces[i].Subnets = nil

		for _, sub := range space.Subnets {
			if subnetIDs.Contains(sub.ID) {
				// Indicate that we found the subnet,
				// but don't do anything if it is already in the space.
				found.Add(sub.ID)
				if string(space.Name) != spaceName {
					sub.SpaceID = newSpace.ID
					sub.SpaceName = spaceName
					sub.ProviderSpaceId = newSpace.ProviderId
					movers = append(movers, sub)
				}
				continue
			}
			newSpaces[i].Subnets = append(newSpaces[i].Subnets, sub)
		}
	}

	// Ensure that the input did not include subnets not in this collection.
	if diff := subnetIDs.Difference(found); len(diff) != 0 {
		return nil, errors.NotFoundf("subnet IDs %v", diff.SortedValues())
	}

	// Then put them against the new one.
	// We have to find the space again in this collection,
	// because newSpace was returned from a copy.
	for i, space := range newSpaces {
		if string(space.Name) == spaceName {
			newSpaces[i].Subnets = append(space.Subnets, movers...)
			break
		}
	}

	return newSpaces, nil
}

// String returns returns a quoted, comma-delimited names of the spaces in the
// collection, or <none> if the collection is empty.
func (s SpaceInfos) String() string {
	if len(s) == 0 {
		return "<none>"
	}
	names := make([]string, len(s))
	for i, v := range s {
		names[i] = fmt.Sprintf("%q", string(v.Name))
	}
	return strings.Join(names, ", ")
}

// Names returns a string slice with each of the space names in the collection.
func (s SpaceInfos) Names() []string {
	names := make([]string, len(s))
	for i, v := range s {
		names[i] = string(v.Name)
	}
	return names
}

// IDs returns a string slice with each of the space ids in the collection.
func (s SpaceInfos) IDs() []string {
	ids := make([]string, len(s))
	for i, v := range s {
		ids[i] = v.ID
	}
	return ids
}

// GetByID returns a reference to the space with the input ID
// if it exists in the collection. Otherwise nil is returned.
func (s SpaceInfos) GetByID(id string) *SpaceInfo {
	for _, space := range s {
		if space.ID == id {
			return &space
		}
	}
	return nil
}

// GetByName returns a reference to the space with the input name
// if it exists in the collection. Otherwise nil is returned.
func (s SpaceInfos) GetByName(name string) *SpaceInfo {
	for _, space := range s {
		if string(space.Name) == name {
			return &space
		}
	}
	return nil
}

// ContainsID returns true if the collection contains a
// space with the given ID.
func (s SpaceInfos) ContainsID(id string) bool {
	return s.GetByID(id) != nil
}

// ContainsName returns true if the collection contains a
// space with the given name.
func (s SpaceInfos) ContainsName(name string) bool {
	return s.GetByName(name) != nil
}

// Minus returns a new SpaceInfos representing all the
// values in the target that are not in the parameter.
// Value matching is done by ID.
func (s SpaceInfos) Minus(other SpaceInfos) SpaceInfos {
	result := make(SpaceInfos, 0)
	for _, value := range s {
		if !other.ContainsID(value.ID) {
			result = append(result, value)
		}
	}
	return result
}

func (s SpaceInfos) InferSpaceFromAddress(addr string) (*SpaceInfo, error) {
	var (
		ip    = net.ParseIP(addr)
		match *SpaceInfo
	)

nextSpace:
	for spIndex, space := range s {
		for _, subnet := range space.Subnets {
			ipNet, err := subnet.ParsedCIDRNetwork()
			if err != nil {
				// Subnets should always have a valid CIDR
				return nil, errors.Trace(err)
			}

			if ipNet.Contains(ip) {
				if match == nil {
					match = &s[spIndex]

					// We still need to check other spaces
					// in case we have multiple networks
					// with the same subnet CIDRs
					continue nextSpace
				}

				return nil, errors.Errorf(
					"unable to infer space for address %q: address matches the same CIDR in multiple spaces", addr)
			}
		}
	}

	if match == nil {
		return nil, errors.NewNotFound(nil, fmt.Sprintf("unable to infer space for address %q", addr))
	}
	return match, nil
}

func (s SpaceInfos) InferSpaceFromCIDRAndSubnetID(cidr, providerSubnetID string) (*SpaceInfo, error) {
	for _, space := range s {
		for _, subnet := range space.Subnets {
			if subnet.CIDR == cidr && string(subnet.ProviderId) == providerSubnetID {
				return &space, nil
			}
		}
	}

	return nil, errors.NewNotFound(
		nil, fmt.Sprintf("unable to infer space for CIDR %q and provider subnet ID %q", cidr, providerSubnetID))
}

// SubnetCIDRsBySpaceID returns the set of known subnet CIDRs grouped by the
// space ID they belong to.
func (s SpaceInfos) SubnetCIDRsBySpaceID() map[string][]string {
	res := make(map[string][]string)
	for _, space := range s {
		for _, sub := range space.Subnets {
			res[space.ID] = append(res[space.ID], sub.CIDR)
		}
	}
	return res
}

var (
	invalidSpaceNameChars = regexp.MustCompile("[^0-9a-z-]")
	dashPrefix            = regexp.MustCompile("^-*")
	dashSuffix            = regexp.MustCompile("-*$")
	multipleDashes        = regexp.MustCompile("--+")
)

// ConvertSpaceName is used to massage provider-sourced (i.e. MAAS)
// space names so that they conform to Juju's space name rules.
func ConvertSpaceName(name string, existing set.Strings) string {
	// Lower case and replace spaces with dashes.
	name = strings.Replace(name, " ", "-", -1)
	name = strings.ToLower(name)

	// Remove any character not in the set "-", "a-z", "0-9".
	name = invalidSpaceNameChars.ReplaceAllString(name, "")

	// Remove any dashes at the beginning and end.
	name = dashPrefix.ReplaceAllString(name, "")
	name = dashSuffix.ReplaceAllString(name, "")

	// Replace multiple dashes with a single dash.
	name = multipleDashes.ReplaceAllString(name, "-")

	// If the name had only invalid characters, give it a new name.
	if name == "" {
		name = "empty"
	}

	// If this name is in use add a numerical suffix.
	if existing.Contains(name) {
		counter := 2
		for existing.Contains(fmt.Sprintf("%s-%d", name, counter)) {
			counter++
		}
		name = fmt.Sprintf("%s-%d", name, counter)
	}

	return name
}
