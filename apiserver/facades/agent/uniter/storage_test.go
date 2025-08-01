// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package uniter_test

import (
	stdtesting "testing"

	"github.com/juju/tc"

	"github.com/juju/juju/internal/testing"
)

type storageSuite struct {
	testing.BaseSuite
}

func TestStorageSuite(t *stdtesting.T) {
	tc.Run(t, &storageSuite{})
}

func (s *storageSuite) TestStub(c *tc.C) {
	c.Skip(`This suite is missing the following tests:
- TestWatchUnitStorageAttachments
- TestWatchStorageAttachmentVolume
- TestCAASWatchStorageAttachmentFilesystem
- TestIAASWatchStorageAttachmentFilesystem
- TestDestroyUnitStorageAttachments
- TestWatchStorageAttachmentVolumeAttachmentChanges
- TestWatchStorageAttachmentStorageAttachmentChanges
- TestWatchStorageAttachmentBlockDevicesChange
`)
}
