// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage

import (
	"testing"

	"github.com/juju/tc"
)

// storageAttachmentUUIDSuite is a suite of tests for asserting the behaviour of
// [StorageAttachmentUUID].
type storageAttachmentUUIDSuite struct{}

// TeststorageAttachmentUUIDSuite runs all of the tests contained within
// [storageAttachmentUUIDSuite].
func TestStorageAttachmentUUIDSuite(t *testing.T) {
	tc.Run(t, storageAttachmentUUIDSuite{})
}

// TestNew tests that constructing a new storageattachment uuid suceeds with no
// errors and the end result is valid.
func (storageAttachmentUUIDSuite) TestNew(c *tc.C) {
	u, err := NewStorageAttachmentUUID()
	c.Check(err, tc.ErrorIsNil)
	c.Check(u.Validate(), tc.ErrorIsNil)
}

// TestStringer asserts the [fmt.Stringer] interface of [StorageAttachmentUUID]
// by making sure the correct string representation of the uuid is returned to
// the caller.
func (storageAttachmentUUIDSuite) TestStringer(c *tc.C) {
	const validUUID = "0de7ed80-bfcf-49b1-876a-31462e940ca1"
	c.Check(StorageAttachmentUUID(validUUID).String(), tc.Equals, validUUID)
}

// TestValidate asserts that a valid uuid passes validation with no errors.
func (storageAttachmentUUIDSuite) TestValidate(c *tc.C) {
	const validUUID = "0de7ed80-bfcf-49b1-876a-31462e940ca1"
	c.Check(StorageAttachmentUUID(validUUID).Validate(), tc.ErrorIsNil)
}

// TestValidateFail asserts that a bad uuid fails validation.
func (storageAttachmentUUIDSuite) TestValidateFail(c *tc.C) {
	c.Check(StorageAttachmentUUID("invalid").Validate(), tc.NotNil)
}
