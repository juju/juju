// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package status

import (
	"strconv"
	"testing"

	"github.com/juju/tc"

	corestatus "github.com/juju/juju/core/status"
	schematesting "github.com/juju/juju/domain/schema/testing"
	statuserrors "github.com/juju/juju/domain/status/errors"
)

// filesystemStatusDDLSuite is a suite of tests for asserting alignment between
// filesystem status enums and the values contained in the model database.
type filesystemStatusDDLSuite struct {
	schematesting.ModelSuite
}

// filesystemStatusSuite is a suite of tests for asserting the behaviour of
// filesystem status types in this package.
type filesystemStatusSuite struct{}

// TestFilesystemStatusDDLSuite runs all of the tests contained within
// [filesystemStatusDDLSuite].
func TestFilesystemStatusDDLSuite(t *testing.T) {
	tc.Run(t, &filesystemStatusDDLSuite{})
}

// TestFilesystemStatusSuite runs all of the tests contained within
// [filesystemStatusSuite].
func TestFilesystemStatusSuite(t *testing.T) {
	tc.Run(t, filesystemStatusSuite{})
}

// TestFilesystemStatusDBValues ensures there's no skew between what's in the
// database table for filesystem status and the typed consts used in the
// state packages.
func (s *filesystemStatusDDLSuite) TestFilesystemStatusDBValues(c *tc.C) {
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
		StorageFilesystemStatusTypeTombstone:  "tombstone",
	})
}

// TestEncodeDecodeFilesystemStatus ensures that Encode and Decode functions
// correctly roundtrip and are consistent with the db lookup values.
func (s *filesystemStatusDDLSuite) TestEncodeDecodeFilesystemStatus(c *tc.C) {
	db := s.DB()
	rows, err := db.Query("SELECT id FROM storage_filesystem_status_value")
	c.Assert(err, tc.ErrorIsNil)
	defer func() { _ = rows.Close() }()

	var (
		dbID   int
		idVals []int
	)
	for rows.Next() {
		err := rows.Scan(&dbID)
		c.Assert(err, tc.ErrorIsNil)
		idVals = append(idVals, dbID)
	}

	for _, id := range idVals {
		c.Run(strconv.Itoa(id), func(c *testing.T) {
			decoded, err := DecodeStorageFilesystemStatus(id)
			tc.Assert(c, err, tc.ErrorIsNil)
			encoded, err := EncodeStorageFilesystemStatus(decoded)
			tc.Assert(c, err, tc.ErrorIsNil)
			tc.Check(c, encoded, tc.Equals, id)
		})
	}
}

// TestToCoreStatus tests that [StorageFilesystemStatusType.ToCoreStatus]
// correctly converts each storage filesystem status type to its corresponding
// core status value. The test verifies all status types except Tombstone, which
// maps to Unknown.
func (filesystemStatusSuite) TestToCoreStatus(c *tc.C) {
	tests := []struct {
		N string
		E corestatus.Status
		T StorageFilesystemStatusType
	}{
		{N: "Pending", E: corestatus.Pending, T: StorageFilesystemStatusTypePending},
		{N: "Error", E: corestatus.Error, T: StorageFilesystemStatusTypeError},
		{N: "Attaching", E: corestatus.Attaching, T: StorageFilesystemStatusTypeAttaching},
		{N: "Attached", E: corestatus.Attached, T: StorageFilesystemStatusTypeAttached},
		{N: "Detaching", E: corestatus.Detaching, T: StorageFilesystemStatusTypeDetaching},
		{N: "Detached", E: corestatus.Detached, T: StorageFilesystemStatusTypeDetached},
		{N: "Destroying", E: corestatus.Destroying, T: StorageFilesystemStatusTypeDestroying},
	}

	for _, test := range tests {
		c.Run(test.N, func(t *testing.T) {
			tc.Check(t, test.T.ToCoreStatus(), tc.Equals, test.E)
		})
	}
}

// TestToCoreStatusUnknown tests that [StorageFilesystemStatusType.ToCoreStatus]
// returns Unknown for unrecognized storage filesystem status type values. This
// verifies the default case behavior in the conversion.
func (filesystemStatusSuite) TestToCoreStatusUnknown(c *tc.C) {
	c.Check(StorageFilesystemStatusType(-100).ToCoreStatus(), tc.Equals, corestatus.Unknown)
}

// TestFilesystemStatusTransitionPendingInvalid tests that
// [FilesystemStatusTransitionValid] returns an error when attempting to
// transition from an attached, provisioned filesystem back to pending status.
// This verifies that invalid backward transitions are rejected.
func (filesystemStatusSuite) TestFilesystemStatusTransitionPendingInvalid(c *tc.C) {
	sts := StatusInfo[StorageFilesystemStatusType]{
		Status: StorageFilesystemStatusTypePending,
	}
	err := FilesystemStatusTransitionValid(
		StorageFilesystemStatusTypeAttached, true, sts)
	c.Assert(err, tc.ErrorIs, statuserrors.FilesystemStatusTransitionNotValid)
}
