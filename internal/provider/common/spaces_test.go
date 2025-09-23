// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common_test

import (
	"testing"

	"github.com/juju/tc"
	jc "github.com/juju/testing/checkers"

	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/internal/provider/common"
)

type SpacesSuite struct{}

func TestSpaceSuite(t *testing.T) {
	tc.Run(t, &SpacesSuite{})
}

func (*SpacesSuite) TestGetValidSubnetZoneMapOneSpaceConstraint(c *tc.C) {
	allSubnetZones := []map[network.Id][]string{
		{network.Id("sub-1"): {"az-1"}},
	}

	args := environs.StartInstanceParams{
		Constraints:    constraints.MustParse("spaces=admin"),
		SubnetsToZones: allSubnetZones,
	}

	subnetZones, err := common.GetValidSubnetZoneMap(c.Context(), args)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(subnetZones, tc.DeepEquals, allSubnetZones[0])
}

func (*SpacesSuite) TestGetValidSubnetZoneMapOneBindingFanFiltered(c *tc.C) {
	allSubnetZones := []map[network.Id][]string{{
		network.Id("sub-1"):       {"az-1"},
		network.Id("sub-INFAN-2"): {"az-2"},
	}}

	args := environs.StartInstanceParams{
		SubnetsToZones: allSubnetZones,
		EndpointBindings: map[string]network.Id{
			"":    "space-1",
			"ep1": "space-1",
			"ep2": "space-1",
		},
	}

	subnetZones, err := common.GetValidSubnetZoneMap(c.Context(), args)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(subnetZones, tc.DeepEquals, map[network.Id][]string{
		"sub-1": {"az-1"},
	})
}

func (*SpacesSuite) TestGetValidSubnetZoneMapNoIntersectionError(c *tc.C) {
	allSubnetZones := []map[network.Id][]string{
		{network.Id("sub-1"): {"az-1"}},
		{network.Id("sub-2"): {"az-2"}},
	}

	args := environs.StartInstanceParams{
		SubnetsToZones: allSubnetZones,
		Constraints:    constraints.MustParse("spaces=admin"),
		EndpointBindings: map[string]network.Id{
			"":    "space-1",
			"ep1": "space-1",
			"ep2": "space-1",
		},
	}

	_, err := common.GetValidSubnetZoneMap(c.Context(), args)
	c.Assert(err, tc.ErrorMatches,
		`unable to satisfy supplied space requirements; spaces: \[admin\], bindings: \[space-1\]`)
}

func (*SpacesSuite) TestGetValidSubnetZoneMapIntersectionSelectsCorrectIndex(c *tc.C) {
	allSubnetZones := []map[network.Id][]string{
		{network.Id("sub-1"): {"az-1"}},
		{network.Id("sub-2"): {"az-2"}},
		{network.Id("sub-3"): {"az-2"}},
	}

	args := environs.StartInstanceParams{
		SubnetsToZones: allSubnetZones,
		Constraints:    constraints.MustParse("spaces=space-2,space-3"),
		EndpointBindings: map[string]network.Id{
			"":    "space-1",
			"ep1": "space-2",
			"ep2": "space-2",
		},
	}

	// space-2 is common to the bindings and constraints and is at index 1
	// of the sorted union.
	// This should result in the selection of the same index from the
	// subnets-to-zones map.

	subnetZones, err := common.GetValidSubnetZoneMap(c.Context(), args)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(subnetZones, tc.DeepEquals, allSubnetZones[1])
}
