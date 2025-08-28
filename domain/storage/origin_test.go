// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage

import (
	"testing"

	"github.com/juju/tc"

	schematesting "github.com/juju/juju/domain/schema/testing"
)

type originSuite struct {
	schematesting.ModelSuite
}

// TestOriginSuite runs all of the tests located in the [originSuite].
func TestOriginSuite(t *testing.T) {
	tc.Run(t, &originSuite{})
}

// TestStoragePoolOriginValuesAligned asserts that the origin values that exist
// in the database schema align with the enum values defined in this package.
//
// If this test fails it indicates that either a new value has been added to the
// schema and a new enum needs to be created or a value has been modified or
// removed that will result in a breaking change.
func (s *originSuite) TestStoragePoolOriginValuesAligned(c *tc.C) {

	rows, err := s.DB().QueryContext(
		c.Context(),
		"SELECT id, origin FROM storage_pool_origin",
	)
	c.Assert(err, tc.ErrorIsNil)
	defer func() { _ = rows.Close() }()

	dbValues := map[StoragePoolOrigin]string{}
	for rows.Next() {
		var (
			id     int
			origin string
		)

		c.Assert(rows.Scan(&id, &origin), tc.ErrorIsNil)
		dbValues[StoragePoolOrigin(id)] = origin
	}

	c.Check(dbValues, tc.DeepEquals, map[StoragePoolOrigin]string{
		StoragePoolOriginUser:            "user",
		StoragePoolOriginProviderDefault: "provider-default",
	})
}
