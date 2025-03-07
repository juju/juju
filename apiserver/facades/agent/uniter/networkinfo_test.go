// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package uniter_test

import (
	"github.com/juju/testing"
	gc "gopkg.in/check.v1"
)

type networkInfoSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&networkInfoSuite{})

func (s *networkInfoSuite) TestStub(c *gc.C) {
	c.Skip(`This suite is missing tests for the following scenarios:
 - TestAPIRequestForRelationCAASHostNameNoIngress: tests api request 
   NetworkInfoForStrategy and make sure the retrieved ingress host address is 
   retrieved, by setting a lookup func with only one host resolvable to an IP 
   addr.
 - TestAPIRequestForRelationIAASHostNameIngressNoEgress: tests api request
   returns invalid response when no egress address is found.
 - TestMachineNetworkInfos returns the network info for the machine. with the
   correct space information.
 - TestNetworksForRelation returns the correct ingress and egress addresses for
   the relation, including the bound space.
 - TestNetworksForRelationCAASModel returns the correct ingress and egress
   addresses for the relation, including the bound space for CAAS.
 - TestNetworksForRelationCAASModelCrossModelNoPrivate returns the correct
   public address, that doesn't contain a private scope.
 - TestNetworksForRelationCAASModelInvalidBinding returns an error when the
   relation binding is not valid.
 - TestNetworksForRelationRemoteRelation returns the correct ingress and egress
   addresses for the relation, including the bound space for remote relations.
 - TestNetworksForRelationRemoteRelationDelayedPrivateAddress returns the
   local address after a retry if fallback isn't found.
 - TestNetworksForRelationRemoteRelationDelayedPublicAddress returns the
   public address after a retry.
 - TestNetworksForRelationRemoteRelationNoPublicAddr returns an error when no
   public address is found, returns local scope.
 - TestNetworksForRelationWithSpaces returns the correct assigned spaces.
 - TestProcessAPIRequestBridgeWithSameIPOverNIC should return the right bridge
   for the NIC.
 - TestProcessAPIRequestForBinding returns the correct binding for the relation.
`)
}
