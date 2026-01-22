// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage

import (
	"testing"

	"github.com/juju/tc"

	internalstorage "github.com/juju/juju/internal/storage"
)

// volumeDeviceTypeSuite tests the contracts offered by [VolumeDeviceType] and
// its constants.
type volumeDeviceTypeSuite struct{}

// TestVolumeDeviceTypeSuite runs the tests contained in
// [volumeDeviceTypeSuite].
func TestVolumeDeviceTypeSuite(t *testing.T) {
	tc.Run(t, volumeDeviceTypeSuite{})
}

// TestStringAlignmentWithInternalStorage is concerned with makeing sure that
// the values returned by [VolumeDeviceType.String] are equivelent to the
// constants defined for [internalstorage.DeviceType].
//
// This allows [VolumeDeviceType] to be used as a replacement till the
// [internalstorage.DeviceType] value can be fully removed.
func (volumeDeviceTypeSuite) TestStringAlignmentWithInternalStorage(c *tc.C) {
	tests := []struct {
		E internalstorage.DeviceType
		V VolumeDeviceType
	}{
		{
			E: internalstorage.DeviceTypeLocal,
			V: VolumeDeviceTypeLocal,
		},
		{
			E: internalstorage.DeviceTypeISCSI,
			V: VolumeDeviceTypeISCSI,
		},
	}

	for _, test := range tests {
		c.Run(test.V.String(), func(c *testing.T) {
			tc.Check(
				c,
				internalstorage.DeviceType(test.V.String()),
				tc.Equals,
				test.E,
			)
		})
	}
}

// TestStringerForUnknownValue asserts that [VolumeDeviceType.String] returns a
// zero value string when the value is not known.
func (volumeDeviceTypeSuite) TestStringerForUnknownValue(c *tc.C) {
	c.Check(VolumeDeviceType(-1).String(), tc.Equals, "")
}
