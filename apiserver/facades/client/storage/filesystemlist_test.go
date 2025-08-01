// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage_test

import (
	"testing"

	"github.com/juju/tc"
)

type filesystemSuite struct {
	baseStorageSuite
}

func TestFilesystemSuite(t *testing.T) {
	tc.Run(t, &filesystemSuite{})
}

func (s *filesystemSuite) TestStub(c *tc.C) {
	c.Skip(`This suite is missing the following tests:
- TestListFilesystemsEmptyFilter
- TestListFilesystemsError
- TestListFilesystemsNoFilesystems
- TestListFilesystemsFilter
- TestListFilesystemsFilterNonMatching
- TestListFilesystemsFilesystemInfo
- TestListFilesystemsAttachmentInfo
- TestListFilesystemsVolumeBacked
	`)
}
