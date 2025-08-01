// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storageprovisioner_test

import (
	stdtesting "testing"

	"github.com/juju/tc"
)

type iaasProvisionerSuite struct {
}

func TestIaasProvisionerSuite(t *stdtesting.T) {
	tc.Run(t, &iaasProvisionerSuite{})
}

func (s *iaasProvisionerSuite) TestStub(c *tc.C) {
	c.Skip(`This suite is missing tests for the following scenarios:
- TestRemoveVolumeParams: creates an app that will create a storage instance,
so we can release the storage and show the effects on the RemoveVolumeParams.
- TestRemoveFilesystemParams: creates an application that will create a storage
instance, so we can release the storage and show the effects on the
RemoveFilesystemParams.
 - Watching volumes triggers a new change
 - Watching volume attachments triggers a new change
 - Watching block devices triggers a new change
 - Watching volume block devices triggers a new change
 - Setting a volume attachment plan block info can be located using volume block devices
 - Test hosted volumes for IAAS
 - Test model volumes
 - Test filesystems
 - Test volume attachments
 - Test filesystem attachments
 - Test volume params
 - Test filesystem params
 - Test volume attachment params
 - Test filesystem attachment params
 - Test set volume attachment info
 - Test set filesystem attachment info
 - Test watch filesystems
 - Test watch volume attachments
 - Test ensure dead for volumes
 - Test remove controller volumes
 - Test remove controller filesystems
 - Test remove volume machine agent
 - Test remove filesystem machine agent
 - Test remove volume machine agent with no attachments
 - Test remove filesystem attachments
`)
}
