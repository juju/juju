// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage

import (
	"testing"

	"github.com/juju/tc"

	schematesting "github.com/juju/juju/domain/schema/testing"
)

type storageKindSuite struct {
	schematesting.ModelSuite
}

func TestStorageKindSuite(t *testing.T) {
	tc.Run(t, &storageKindSuite{})
}

// TestStorageKindDBValues ensures there's no skew between what's in the
// database table for charm_storage_kind and the typed consts used in the state packages.
func (s *storageKindSuite) TestStorageKindDBValues(c *tc.C) {
	db := s.DB()
	rows, err := db.Query("SELECT id, kind FROM charm_storage_kind")
	c.Assert(err, tc.ErrorIsNil)
	defer func() { _ = rows.Close() }()

	dbValues := make(map[StorageKind]string)
	for rows.Next() {
		var (
			id    int
			value string
		)

		c.Assert(rows.Scan(&id, &value), tc.ErrorIsNil)
		dbValues[StorageKind(id)] = value
	}
	c.Assert(dbValues, tc.DeepEquals, map[StorageKind]string{
		StorageKindFilesystem: "filesystem",
		StorageKindBlock:      "block",
	})
}
