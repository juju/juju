// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provisioner_test

import (
	"github.com/juju/tc"
)

func (s *withoutControllerSuite) TestStubSuite(c *tc.C) {
	c.Skip(`This suite is missing tests for the following scenarios:
- ProvisioningInfo where the alpha space is explicitly set for all bindings and
  as a constraint, causing ProvisioningNetworkTopology to have only subnets and
  zones from the alpha space.
- TestStorageProviderFallbackToType: Check ProvisioningInfo for includes volume
  attachment information where one provider is environ-managed and one is not.
- TestStorageProviderVolumes: Check ProvisioningInfo for includes volume
  attachment information for already-provisioned volumes.
- TestProvisioningInfoWithLXDProfile: Check ProvisioningInfo for includes
  CharmLXDProfiles when a unit with an LXD profile is on the machine.
`)
}
