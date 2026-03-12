// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package status

import (
	"strconv"
	"testing"

	"github.com/juju/tc"

	corestatus "github.com/juju/juju/core/status"
	schematesting "github.com/juju/juju/domain/schema/testing"
	domainstatuserrors "github.com/juju/juju/domain/status/errors"
)

// volumeStatusDDLSuite is a suite of tests for asserting alignment between
// volume status enums and the values contained in the model database.
type volumeStatusDDLSuite struct {
	schematesting.ModelSuite
}

// volumeStatusSuite is a suite of tests for asserting the behaviour of volume
// status types in this package.
type volumeStatusSuite struct{}

// TestVolumeStatusDDLSuite runs all of the tests contained within
// [volumeStatusDDLSuite].
func TestVolumeStatusDDLSuite(t *testing.T) {
	tc.Run(t, &volumeStatusDDLSuite{})
}

// TestVolumeStatusSuite runs all of the tests contained within
// [volumeStatusSuite].
func TestVolumeStatusSuite(t *testing.T) {
	tc.Run(t, volumeStatusSuite{})
}

// TestVolumeStatusDBValues ensures there's no skew between what's in the
// database table for volume status and the typed consts used in the
// state packages.
func (s *volumeStatusDDLSuite) TestVolumeStatusDBValues(c *tc.C) {
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
		StorageVolumeStatusTypeTombstone:  "tombstone",
	})
}

// TestEncodeDecodeVolumeStatus ensures that Encode and Decode functions
// correctly roundtrip and are consistent with the db lookup values.
func (s *volumeStatusDDLSuite) TestEncodeDecodeVolumeStatus(c *tc.C) {
	db := s.DB()
	rows, err := db.Query("SELECT id FROM storage_volume_status_value")
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
			decoded, err := DecodeStorageVolumeStatus(id)
			tc.Assert(c, err, tc.ErrorIsNil)
			encoded, err := EncodeStorageVolumeStatus(decoded)
			tc.Assert(c, err, tc.ErrorIsNil)
			tc.Check(c, encoded, tc.Equals, id)
		})
	}
}

// TestToCoreStatus tests that [StorageVolumeStatusType.ToCoreStatus] correctly
// converts each storage volume status type to its corresponding core status
// value. The test verifies all status types except Tombstone, which maps to
// Unknown.
func (volumeStatusSuite) TestToCoreStatus(c *tc.C) {
	tests := []struct {
		N string
		E corestatus.Status
		T StorageVolumeStatusType
	}{
		{N: "Pending", E: corestatus.Pending, T: StorageVolumeStatusTypePending},
		{N: "Error", E: corestatus.Error, T: StorageVolumeStatusTypeError},
		{N: "Attaching", E: corestatus.Attaching, T: StorageVolumeStatusTypeAttaching},
		{N: "Attached", E: corestatus.Attached, T: StorageVolumeStatusTypeAttached},
		{N: "Detaching", E: corestatus.Detaching, T: StorageVolumeStatusTypeDetaching},
		{N: "Detached", E: corestatus.Detached, T: StorageVolumeStatusTypeDetached},
		{N: "Destroying", E: corestatus.Destroying, T: StorageVolumeStatusTypeDestroying},
	}

	for _, test := range tests {
		c.Run(test.N, func(c *testing.T) {
			tc.Check(c, test.T.ToCoreStatus(), tc.Equals, test.E)
		})
	}
}

// TestToCoreStatusUnknown tests that [StorageVolumeStatusType.ToCoreStatus]
// returns Unknown for unrecognized storage volume status type values. This
// verifies the default case behavior in the conversion.
func (volumeStatusSuite) TestToCoreStatusUnknown(c *tc.C) {
	c.Check(
		StorageVolumeStatusType(-100).ToCoreStatus(),
		tc.Equals,
		corestatus.Unknown,
	)
}

// TestVolumeStatusTransitionPendingInvalid tests that
// [VolumeStatusTransitionValid] returns an error when attempting to transition
// from an attached, provisioned volume back to pending status. This verifies
// that invalid backward transitions are rejected.
func (volumeStatusSuite) TestVolumeStatusTransitionPendingInvalid(c *tc.C) {
	sts := StatusInfo[StorageVolumeStatusType]{
		Status: StorageVolumeStatusTypePending,
	}
	err := VolumeStatusTransitionValid(
		StorageVolumeStatusTypeAttached, true, sts)
	c.Assert(err, tc.ErrorIs, domainstatuserrors.VolumeStatusTransitionNotValid)
}
