// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storageprovisioner

import (
	"testing"

	"github.com/juju/tc"
)

type caasProvisionerSuite struct {
}

func TestCaasProvisionerSuite(t *testing.T) {
	tc.Run(t, &caasProvisionerSuite{})
}

func (s *caasProvisionerSuite) TestStub(c *tc.C) {
	c.Skip(`This suite is missing tests for the following scenarios:
- TestRemoveVolumeAttachment: remove volume attachments.
- TestRemoveFilesystemAttachments: remove filesystem attachments.
- TestFilesystemAttachmentLife: get life of filesystem attachments.
- TestVolumeAttachmentLife: get life of volume attachments.
`)
}
