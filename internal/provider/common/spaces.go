// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common

import (
	"sort"

	"github.com/juju/collections/set"
	"github.com/juju/errors"

	"github.com/juju/juju/core/network"
	"github.com/juju/juju/environs"
)

// GetValidSubnetZoneMap ensures that (a single one of) any supplied space
// requirements are congruent and can be met, and that the representative
// subnet-zone map is returned, with Fan networks filtered out.
// The returned map will be nil if there are no space requirements.
func GetValidSubnetZoneMap(args environs.StartInstanceParams) (map[network.Id][]string, error) {
	spaceCons := args.Constraints.IncludeSpaces()

	bindings := set.NewStrings()
	for _, spaceName := range args.EndpointBindings {
		bindings.Add(spaceName.String())
	}

	conCount := len(spaceCons)
	bindCount := len(bindings)

	// If there are no bindings or space constraints, we have no limitations
	// and should not have even received start arguments with a subnet/zone
	// mapping - just return nil and attempt provisioning in the current AZ.
	if conCount == 0 && bindCount == 0 {
		return nil, nil
	}

	sort.Strings(spaceCons)
	allSpaceReqs := bindings.Union(set.NewStrings(spaceCons...)).SortedValues()

	// We only need to validate if both bindings and constraints are present.
	// If one is supplied without the other, we know that the value for
	// args.SubnetsToZones correctly reflects the set of spaces.
	var indexInCommon int
	if conCount > 0 && bindCount > 0 {
		// If we have spaces in common between bindings and constraints,
		// the union count will be fewer than the sum.
		// If it is not, just error out here.
		if len(allSpaceReqs) == conCount+bindCount {
			return nil, errors.Errorf("unable to satisfy supplied space requirements; spaces: %v, bindings: %v",
				spaceCons, bindings.SortedValues())
		}

		// Now get the first index of the space in common.
		for _, conSpaceName := range spaceCons {
			if !bindings.Contains(conSpaceName) {
				continue
			}

			for i, spaceName := range allSpaceReqs {
				if conSpaceName == spaceName {
					indexInCommon = i
					break
				}
			}
		}
	}

	// TODO (manadart 2020-02-07): We only take a single subnet/zones
	// mapping to create a NIC for the instance.
	// This is behaviour that dates from the original spaces MVP.
	// It will not take too much effort to enable multi-NIC support for
	// providers which support multi-nic if we use them all when
	// constructing the instance creation request.
	if conCount > 1 || bindCount > 1 {
		logger.Warningf("only considering the space requirement for %q", allSpaceReqs[indexInCommon])
	}

	// We should always have a mapping if there are space requirements,
	// and it should always have the same length as the union of
	// constraints + bindings.
	// However unlikely, rather than taking this for granted and possibly
	// panicking, log a warning and let the provisioning continue.
	mappingCount := len(args.SubnetsToZones)
	if mappingCount == 0 || mappingCount <= indexInCommon {
		logger.Warningf(
			"got space requirements, but not a valid subnet-zone map; constraints/bindings not applied")
		return nil, nil
	}

	// Select the subnet-zone mapping at the index we determined minus Fan
	// networks which we can not consider for provisioning non-containers.
	// We know that the index determined from the spaces union corresponds
	// with the right mapping because of consistent sorting by the provisioner.
	subnetZones := make(map[network.Id][]string)
	for id, zones := range args.SubnetsToZones[indexInCommon] {
		if !network.IsInFanNetwork(id) {
			subnetZones[id] = zones
		}
	}

	return subnetZones, nil
}
