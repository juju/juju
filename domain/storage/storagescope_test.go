// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage

import (
	"testing"

	"github.com/juju/tc"

	schematesting "github.com/juju/juju/domain/schema/testing"
)

type storageScope struct {
	schematesting.ModelSuite
}

func TestStorageScope(t *testing.T) {
	tc.Run(t, &storageScope{})
}

// TestStorageScopeDBValues ensures there's no skew between what's in the
// database table for storage_scope and the typed consts used in the storage domain.
func (s *storageScope) TestStorageScopeDBValues(c *tc.C) {
	db := s.DB()
	rows, err := db.Query("SELECT id, scope FROM storage_scope")
	c.Assert(err, tc.ErrorIsNil)
	defer func() { _ = rows.Close() }()

	dbValues := make(map[StorageScope]string)
	for rows.Next() {
		var (
			id    int
			value string
		)

		c.Assert(rows.Scan(&id, &value), tc.ErrorIsNil)
		dbValues[StorageScope(id)] = value
	}
	c.Assert(dbValues, tc.DeepEquals, map[StorageScope]string{
		StorageScopeModel: "model",
		StorageScopeHost:  "host",
	})
}
