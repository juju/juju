// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provisioner_test

import (
	"github.com/juju/tc"
)

func (s *withoutControllerSuite) TestStubSuite(c *tc.C) {
	c.Skip(`This suite is missing tests for the following scenarios:
- ProvisioningInfo for endpoint bindings to multiple spaces.
  In particular the EndpointBindings and ProvisioningNetworkTopology should be
  populated and included relevant space, subnet and zone data.
- ProvisioningInfo where there are endpoint bindings, but the alpha space has no
  subnets/zones, which causes an error saying it cannot be used as a deployment
  target due to having no subnets.
- ProvisioningInfo where a negative space constraint conflicts with endpoint
  bindings causing an error to be returned.
- ProvisioningInfo where the alpha space is explicitly set for all bindings and
  as a constraint, causing ProvisioningNetworkTopology to have only subnets and
  zones from the alpha space.
- TestProvisioningInfoWithStorage: Check ProvisioningInfo for includes volume
  attachment information
- TestProvisioningInfoRootDiskVolume: Check ProvisioningInfo for includes root
  disk information
- TestProvisioningInfoWithMultiplePositiveSpaceConstraints: Check ProvisioningInfo
  includes network topology information
- TestProvisioningInfoWithUnsuitableSpacesConstraints: Check ProvisioningInfo
  returns errors for spaces constraints that are not suitable for deployment
  target
- TestStorageProviderFallbackToType: Check ProvisioningInfo for includes volume
  attachment information
- TestStorageProviderVolumes: Check ProvisioningInfo for includes volume
  attachment information
- TestProviderInfoCloudInitUserData: Check ProvisioningInfo for includes cloud
  init user data
- TestProvisioningInfoPermissions: Check ProvisioningInfo for permissions
`)
}
