// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package status

import (
	"testing"

	"github.com/juju/tc"

	schematesting "github.com/juju/juju/domain/schema/testing"
	statuserrors "github.com/juju/juju/domain/status/errors"
)

type storageStatusSuite struct {
	schematesting.ModelSuite
}

func TestStorageStatusSuite(t *testing.T) {
	tc.Run(t, &storageStatusSuite{})
}

// TestFilesystemStatusDBValues ensures there's no skew between what's in the
// database table for filesystem status and the typed consts used in the
// state packages.
func (s *storageStatusSuite) TestFilesystemStatusDBValues(c *tc.C) {
	db := s.DB()
	rows, err := db.Query("SELECT id, status FROM storage_filesystem_status_value")
	c.Assert(err, tc.ErrorIsNil)
	defer func() { _ = rows.Close() }()

	dbValues := make(map[StorageFilesystemStatusType]string)
	for rows.Next() {
		var (
			id   int
			name string
		)
		err := rows.Scan(&id, &name)
		c.Assert(err, tc.ErrorIsNil)
		dbValues[StorageFilesystemStatusType(id)] = name
	}
	c.Assert(dbValues, tc.DeepEquals, map[StorageFilesystemStatusType]string{
		StorageFilesystemStatusTypePending:    "pending",
		StorageFilesystemStatusTypeError:      "error",
		StorageFilesystemStatusTypeAttaching:  "attaching",
		StorageFilesystemStatusTypeAttached:   "attached",
		StorageFilesystemStatusTypeDetaching:  "detaching",
		StorageFilesystemStatusTypeDetached:   "detached",
		StorageFilesystemStatusTypeDestroying: "destroying",
	})
}

// TestEncodeDecodeFilesystemStatus ensures that Encode and Decode functions
// correctly roundtrip and are consistent with the db lookup values.
func (s *storageStatusSuite) TestEncodeDecodeFilesystemStatus(c *tc.C) {
	db := s.DB()
	rows, err := db.Query("SELECT id FROM storage_filesystem_status_value")
	c.Assert(err, tc.ErrorIsNil)
	defer func() { _ = rows.Close() }()

	for rows.Next() {
		var id int
		err := rows.Scan(&id)
		c.Assert(err, tc.ErrorIsNil)
		decoded, err := DecodeStorageFilesystemStatus(id)
		c.Assert(err, tc.ErrorIsNil)
		encoded, err := EncodeStorageFilesystemStatus(decoded)
		c.Assert(err, tc.ErrorIsNil)
		c.Assert(encoded, tc.Equals, id)
	}
}

func (s *storageStatusSuite) TestFilesystemStatusTransitionErrorInvalid(c *tc.C) {
	sts := StatusInfo[StorageFilesystemStatusType]{
		Status: StorageFilesystemStatusTypeError,
	}
	err := FilesystemStatusTransitionValid(
		StorageFilesystemStatusTypeAttached, true, sts)
	c.Assert(err, tc.ErrorIs, statuserrors.FilesystemStatusTransitionNotValid)
}

func (s *storageStatusSuite) TestFilesystemStatusTransitionPendingInvalid(c *tc.C) {
	sts := StatusInfo[StorageFilesystemStatusType]{
		Status: StorageFilesystemStatusTypePending,
	}
	err := FilesystemStatusTransitionValid(
		StorageFilesystemStatusTypeAttached, true, sts)
	c.Assert(err, tc.ErrorIs, statuserrors.FilesystemStatusTransitionNotValid)
}

// TestVolumeStatusDBValues ensures there's no skew between what's in the
// database table for volume status and the typed consts used in the
// state packages.
func (s *storageStatusSuite) TestVolumeStatusDBValues(c *tc.C) {
	db := s.DB()
	rows, err := db.Query("SELECT id, status FROM storage_volume_status_value")
	c.Assert(err, tc.ErrorIsNil)
	defer func() { _ = rows.Close() }()

	dbValues := make(map[StorageVolumeStatusType]string)
	for rows.Next() {
		var (
			id   int
			name string
		)
		err := rows.Scan(&id, &name)
		c.Assert(err, tc.ErrorIsNil)
		dbValues[StorageVolumeStatusType(id)] = name
	}
	c.Assert(dbValues, tc.DeepEquals, map[StorageVolumeStatusType]string{
		StorageVolumeStatusTypePending:    "pending",
		StorageVolumeStatusTypeError:      "error",
		StorageVolumeStatusTypeAttaching:  "attaching",
		StorageVolumeStatusTypeAttached:   "attached",
		StorageVolumeStatusTypeDetaching:  "detaching",
		StorageVolumeStatusTypeDetached:   "detached",
		StorageVolumeStatusTypeDestroying: "destroying",
	})
}

// TestEncodeDecodeVolumeStatus ensures that Encode and Decode functions
// correctly roundtrip and are consistent with the db lookup values.
func (s *storageStatusSuite) TestEncodeDecodeVolumeStatus(c *tc.C) {
	db := s.DB()
	rows, err := db.Query("SELECT id FROM storage_volume_status_value")
	c.Assert(err, tc.ErrorIsNil)
	defer func() { _ = rows.Close() }()

	for rows.Next() {
		var id int
		err := rows.Scan(&id)
		c.Assert(err, tc.ErrorIsNil)
		decoded, err := DecodeStorageVolumeStatus(id)
		c.Assert(err, tc.ErrorIsNil)
		encoded, err := EncodeStorageVolumeStatus(decoded)
		c.Assert(err, tc.ErrorIsNil)
		c.Assert(encoded, tc.Equals, id)
	}
}

func (s *storageStatusSuite) TestVolumeStatusTransitionErrorInvalid(c *tc.C) {
	sts := StatusInfo[StorageVolumeStatusType]{
		Status: StorageVolumeStatusTypeError,
	}
	err := VolumeStatusTransitionValid(
		StorageVolumeStatusTypeAttached, true, sts)
	c.Assert(err, tc.ErrorIs, statuserrors.VolumeStatusTransitionNotValid)
}

func (s *storageStatusSuite) TestVolumeStatusTransitionPendingInvalid(c *tc.C) {
	sts := StatusInfo[StorageVolumeStatusType]{
		Status: StorageVolumeStatusTypePending,
	}
	err := VolumeStatusTransitionValid(
		StorageVolumeStatusTypeAttached, true, sts)
	c.Assert(err, tc.ErrorIs, statuserrors.VolumeStatusTransitionNotValid)
}
