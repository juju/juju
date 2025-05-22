// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package filesystem

import (
	"testing"

	"github.com/juju/tc"

	schematesting "github.com/juju/juju/domain/schema/testing"
)

type filesystemtypeSuite struct {
	schematesting.ModelSuite
}

func TestFilesystemtypeSuite(t *testing.T) {
	tc.Run(t, &filesystemtypeSuite{})
}

// TestFilesystemTypeDBValues ensures there's no skew between what's in the
// database table for filesystem type and the typed consts used in the state packages.
func (s *filesystemtypeSuite) TestFilesystemTypeDBValues(c *tc.C) {
	db := s.DB()
	rows, err := db.Query("SELECT id, name FROM filesystem_type")
	c.Assert(err, tc.ErrorIsNil)
	defer rows.Close()

	dbValues := make(map[FilesystemType]string)
	for rows.Next() {
		var (
			id    int
			value string
		)
		err := rows.Scan(&id, &value)
		c.Assert(err, tc.ErrorIsNil)
		dbValues[FilesystemType(id)] = value
	}
	c.Assert(dbValues, tc.DeepEquals, map[FilesystemType]string{
		Unspecified: "unspecified",
		Vfat:        "vfat",
		Ext4:        "ext4",
		Xfs:         "xfs",
		Btrfs:       "btrfs",
		Zfs:         "zfs",
		Jfs:         "jfs",
		Squashfs:    "squashfs",
		Bcachefs:    "bcachefs",
	})
}
