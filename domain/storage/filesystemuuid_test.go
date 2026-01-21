// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage

import (
	"testing"

	"github.com/juju/tc"
)

// filesystemUUIDSuite is a suite of tests for asserting the behaviour of
// [filesystemUUID].
type filesystemUUIDSuite struct{}

// TestfilesystemUUIDSuite runs all of the tests contained within [filesystemUUIDSuite].
func TestFilesystemUUIDSuite(t *testing.T) {
	tc.Run(t, filesystemUUIDSuite{})
}

// TestNew tests that constructing a new filesystem uuid suceeds with no errors and
// the end result is valid.
func (filesystemUUIDSuite) TestNew(c *tc.C) {
	u, err := NewFilesystemUUID()
	c.Check(err, tc.ErrorIsNil)
	c.Check(u.Validate(), tc.ErrorIsNil)
}

// TestStringer asserts the [fmt.Stringer] interface of [FilesystemUUID] by making
// sure the correct string representation of the uuid is returned to the caller.
func (filesystemUUIDSuite) TestStringer(c *tc.C) {
	const validUUID = "0de7ed80-bfcf-49b1-876a-31462e940ca1"
	c.Check(FilesystemUUID(validUUID).String(), tc.Equals, validUUID)
}

// TestValidate asserts that a valid uuid passes validation with no errors.
func (filesystemUUIDSuite) TestValidate(c *tc.C) {
	const validUUID = "0de7ed80-bfcf-49b1-876a-31462e940ca1"
	c.Check(FilesystemUUID(validUUID).Validate(), tc.ErrorIsNil)
}

// TestValidateFail asserts that a bad uuid fails validation.
func (filesystemUUIDSuite) TestValidateFail(c *tc.C) {
	c.Check(FilesystemUUID("invalid").Validate(), tc.NotNil)
}
