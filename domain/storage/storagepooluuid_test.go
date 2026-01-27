// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage

import (
	"testing"

	"github.com/juju/tc"
)

// storagePoolUUIDSuite is a set of tests for asserting the functionality on
// offer by [StoragePoolUUID].
type storagePoolUUIDSuite struct{}

// TestStoragePoolUUIDSuite runs all of the tests contained within
// [storagePoolUUIDSuite].
func TestStoragePoolUUIDSuite(t *testing.T) {
	tc.Run(t, storagePoolUUIDSuite{})
}

// TestNew tests that constructing a new storagepool uuid succeeds with no
// errors and the end result is valid.
func (storagePoolUUIDSuite) TestNew(c *tc.C) {
	u, err := NewStoragePoolUUID()
	c.Check(err, tc.ErrorIsNil)
	c.Check(u.Validate(), tc.ErrorIsNil)
}

// TestStringer asserts the [fmt.Stringer] interface of [StoragePoolUUID] by
// making sure the correct string representation of the uuid is returned to the
// caller.
func (storagePoolUUIDSuite) TestStringer(c *tc.C) {
	const validUUID = "0de7ed80-bfcf-49b1-876a-31462e940ca1"
	c.Check(StoragePoolUUID(validUUID).String(), tc.Equals, validUUID)
}

// TestValidate asserts that a valid uuid passes validation with no errors.
func (storagePoolUUIDSuite) TestValidate(c *tc.C) {
	const validUUID = "0de7ed80-bfcf-49b1-876a-31462e940ca1"
	c.Check(StoragePoolUUID(validUUID).Validate(), tc.ErrorIsNil)
}

// TestValidateFail asserts that a bad uuid fails validation.
func (storagePoolUUIDSuite) TestValidateFail(c *tc.C) {
	c.Check(StoragePoolUUID("invalid").Validate(), tc.NotNil)
}
