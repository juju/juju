// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage

import (
	"testing"

	"github.com/juju/tc"

	schematesting "github.com/juju/juju/domain/schema/testing"
)

// volumeDDLSuite is a set of tests concerned with making sure that types
// defined for volumes in this package align to that of the model database.
// Examples are enum value s.
type volumeDDLSuite struct {
	schematesting.ModelSuite
}

// TestVolumeDDLSuite runs all of the tests contained within [volumeDDLSuite].
func TestVolumeDDLSuite(t *testing.T) {
	tc.Run(t, &volumeDDLSuite{})
}

// TestVolumeDeviceTypeValuesAgainstDDL ensures that the database values defined
// in storage_volume_device_type aligns with the enums in this package.
func (s *volumeDDLSuite) TestVolumeDeviceTypeValuesAgainstDDL(c *tc.C) {
	rows, err := s.DB().QueryContext(
		c.Context(),
		"SELECT id, name FROM storage_volume_device_type",
	)
	c.Assert(err, tc.ErrorIsNil)
	defer rows.Close()

	type volumeDeviceType struct {
		ID   int
		Name string
	}

	var row volumeDeviceType
	var volumeDeviceTypes []volumeDeviceType
	for rows.Next() {
		err := rows.Scan(&row.ID, &row.Name)
		c.Assert(err, tc.ErrorIsNil)
		volumeDeviceTypes = append(volumeDeviceTypes, row)
	}
	c.Assert(rows.Err(), tc.ErrorIsNil)

	c.Check(volumeDeviceTypes, tc.SameContents, []volumeDeviceType{
		{
			ID:   int(VolumeDeviceTypeISCSI),
			Name: VolumeDeviceTypeISCSI.String(),
		},
		{
			ID:   int(VolumeDeviceTypeLocal),
			Name: VolumeDeviceTypeLocal.String(),
		},
	})
}
