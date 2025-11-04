// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charm

import (
	"testing"

	"github.com/juju/tc"

	internalcharm "github.com/juju/juju/internal/charm"
)

// storageTypeSuite is a test suite for asserting the charm storage value
// contained in this package.
type storageTypeSuite struct{}

// TestStorageTypeSuite runs the tests defined in [storageTypeSuite].
func TestStorageTypeSuite(t *testing.T) {
	tc.Run(t, &storageTypeSuite{})
}

// TestStorageTypeMatchesInternalCharm asserts that the storage enums defined
// in this package have the same value as those in [internalcharm.StorageType].
//
// This is a sanity test to make sure that no scew occurs over time.
func (s *storageTypeSuite) TestStorageTypeMatchesInternalCharm(c *tc.C) {
	c.Check(StorageFilesystem.String(), tc.Equals, internalcharm.StorageFilesystem.String())
	c.Check(StorageBlock.String(), tc.Equals, internalcharm.StorageBlock.String())
}
