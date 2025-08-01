// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage_test

import (
	"testing"

	"github.com/juju/tc"
)

type volumeSuite struct {
	baseStorageSuite
}

func TestVolumeSuite(t *testing.T) {
	tc.Run(t, &volumeSuite{})
}

func (s *volumeSuite) TestStub(c *tc.C) {
	c.Skip(`This suite is missing the following tests:
- TestListVolumesNoFilters
- TestListVolumesEmptyFilter
- TestListVolumesError
- TestListVolumesNoVolumes
- TestListVolumesFilter
- TestListVolumesFilterNonMatching
- TestListVolumesVolumeInfo
- TestListVolumesAttachmentInfo
- TestListVolumesStorageLocationBlockDevicePath
- TestListVolumesStorageLocationNoBlockDevice
`)
}
