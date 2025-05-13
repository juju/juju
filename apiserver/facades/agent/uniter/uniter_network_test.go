// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package uniter_test

import "github.com/juju/tc"

type uniterNetworkInfoSuite struct{}

var _ = tc.Suite(&uniterNetworkInfoSuite{})

func (s *uniterNetworkInfoSuite) TestStub(c *tc.C) {
	c.Skip(`This suite had tests with the following names. To re-ensure coverage, check the 3.6 branch for definitions:
- TestNetworkInfoPermissions
- TestNetworkInfoSpaceless
- TestNetworkInfoForExplicitlyBoundEndpointAndDefaultSpace
- TestNetworkInfoForImplicitlyBoundEndpoint
- TestNetworkInfoUsesRelationAddressNonDefaultBinding
- TestNetworkInfoUsesRelationAddressDefaultBinding
- TestUpdateNetworkInfo
- TestCommitHookChanges (should never have been added to this suite)
- TestCommitHookChangesWhenNotLeader (should never have been added to this suite)
`)
}
