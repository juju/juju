// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage

import (
	"testing"

	"github.com/juju/tc"
)

// filesystemAttachmentUUIDSuite is a suite of tests for asserting the behaviour of
// [FilesystemAttachmentUUID].
type filesystemAttachmentUUIDSuite struct{}

// TestFilesystemAttachmentUUIDSuite runs all of the tests contained within
// [filesystemAttachmentUUIDSuite].
func TestFilesystemAttachmentUUIDSuite(t *testing.T) {
	tc.Run(t, filesystemAttachmentUUIDSuite{})
}

// TestNew tests that constructing a new [FilesystemAttachmentUUID] succeeds with
// no errors and the end result is valid.
func (filesystemAttachmentUUIDSuite) TestNew(c *tc.C) {
	u, err := NewFilesystemAttachmentUUID()
	c.Check(err, tc.ErrorIsNil)
	c.Check(u.Validate(), tc.ErrorIsNil)
}

// TestStringer asserts the [fmt.Stringer] interface of
// [FilesystemAttachmentUUID] by making sure the correct string representation
// of the uuid is returned to the caller.
func (filesystemAttachmentUUIDSuite) TestStringer(c *tc.C) {
	const validUUID = "0de7ed80-bfcf-49b1-876a-31462e940ca1"
	c.Check(FilesystemAttachmentUUID(validUUID).String(), tc.Equals, validUUID)
}

// TestValidate asserts that a valid uuid passes validation with no errors.
func (filesystemAttachmentUUIDSuite) TestValidate(c *tc.C) {
	const validUUID = "0de7ed80-bfcf-49b1-876a-31462e940ca1"
	c.Check(FilesystemAttachmentUUID(validUUID).Validate(), tc.ErrorIsNil)
}

// TestValidateFail asserts that a bad uuid fails validation.
func (filesystemAttachmentUUIDSuite) TestValidateFail(c *tc.C) {
	c.Check(FilesystemAttachmentUUID("invalid").Validate(), tc.NotNil)
}
