// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage

import (
	"testing"

	"github.com/juju/tc"

	"github.com/juju/juju/domain/application/internal"
	domainstorage "github.com/juju/juju/domain/storage"
)

// attachSizeSuite is a suite of tests for asserting the behaviour of
// [CalculateStorageInstanceSizeForAttachment].
type attachSizeSuite struct{}

// TestAttachSizeSuite runs the tests contained in [attachSizeSuite].
func TestAttachSizeSuite(t *testing.T) {
	tc.Run(t, &attachSizeSuite{})
}

// TestCalculateSizeFilesystemUsesProvisioned verifies provisioned filesystem
// size takes precedence over requested size.
func (s *attachSizeSuite) TestCalculateSizeFilesystemUsesProvisioned(c *tc.C) {
	info := internal.StorageInstanceInfo{
		Kind:             domainstorage.StorageKindFilesystem,
		RequestedSizeMIB: 10,
		Filesystem: &internal.StorageInstanceFilesystemInfo{
			SizeMib: 20,
		},
	}

	size := CalculateStorageInstanceSizeForAttachment(info)
	c.Check(size, tc.Equals, uint64(20))
}

// TestCalculateSizeFilesystemFallsBackToRequested verifies requested size is
// used when filesystem size is unset.
func (s *attachSizeSuite) TestCalculateSizeFilesystemFallsBackToRequested(c *tc.C) {
	info := internal.StorageInstanceInfo{
		Kind:             domainstorage.StorageKindFilesystem,
		RequestedSizeMIB: 15,
		Filesystem: &internal.StorageInstanceFilesystemInfo{
			SizeMib: 0,
		},
	}

	size := CalculateStorageInstanceSizeForAttachment(info)
	c.Check(size, tc.Equals, uint64(15))
}

// TestCalculateSizeFilesystemWithNoFilesystemInfo verifies nil filesystem info
// still falls back to requested size.
func (s *attachSizeSuite) TestCalculateSizeFilesystemWithNoFilesystemInfo(c *tc.C) {
	info := internal.StorageInstanceInfo{
		Kind:             domainstorage.StorageKindFilesystem,
		RequestedSizeMIB: 12,
	}

	size := CalculateStorageInstanceSizeForAttachment(info)
	c.Check(size, tc.Equals, uint64(12))
}

// TestCalculateSizeBlockUsesProvisioned verifies provisioned volume size takes
// precedence over requested size.
func (s *attachSizeSuite) TestCalculateSizeBlockUsesProvisioned(c *tc.C) {
	info := internal.StorageInstanceInfo{
		Kind:             domainstorage.StorageKindBlock,
		RequestedSizeMIB: 8,
		Volume: &internal.StorageInstanceVolumeInfo{
			SizeMiB: 25,
		},
	}

	size := CalculateStorageInstanceSizeForAttachment(info)
	c.Check(size, tc.Equals, uint64(25))
}

// TestCalculateSizeBlockFallsBackToRequested verifies requested size is used
// when volume size is unset.
func (s *attachSizeSuite) TestCalculateSizeBlockFallsBackToRequested(c *tc.C) {
	info := internal.StorageInstanceInfo{
		Kind:             domainstorage.StorageKindBlock,
		RequestedSizeMIB: 9,
		Volume: &internal.StorageInstanceVolumeInfo{
			SizeMiB: 0,
		},
	}

	size := CalculateStorageInstanceSizeForAttachment(info)
	c.Check(size, tc.Equals, uint64(9))
}

// TestCalculateSizeBlockWithNoVolumeInfo verifies nil volume info still falls
// back to requested size.
func (s *attachSizeSuite) TestCalculateSizeBlockWithNoVolumeInfo(c *tc.C) {
	info := internal.StorageInstanceInfo{
		Kind:             domainstorage.StorageKindBlock,
		RequestedSizeMIB: 11,
	}

	size := CalculateStorageInstanceSizeForAttachment(info)
	c.Check(size, tc.Equals, uint64(11))
}

// TestCalculateSizeUnknownKindUsesRequested verifies unknown kinds default to
// requested size for safety.
func (s *attachSizeSuite) TestCalculateSizeUnknownKindUsesRequested(c *tc.C) {
	info := internal.StorageInstanceInfo{
		Kind:             domainstorage.StorageKind(99),
		RequestedSizeMIB: 7,
	}

	size := CalculateStorageInstanceSizeForAttachment(info)
	c.Check(size, tc.Equals, uint64(7))
}
