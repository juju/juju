// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storageprovisioning

import (
	"testing"

	"github.com/juju/tc"

	schematesting "github.com/juju/juju/domain/schema/testing"
)

type kindSuite struct {
	schematesting.ModelSuite
}

// TestKindSuite runs all of the tests located in the [scopeSuite].
func TestKindSuite(t *testing.T) {
	tc.Run(t, &kindSuite{})
}

// TestStorageKindValuesAligned asserts that the storage kind values that
// exist in the database schema align with the enum values defined in this
// package.
//
// If this test fails it indicates that either a new value has been added to the
// schema and a new enum needs to be created or a value has been modified or
// removed that will result in a breaking change.
func (s *kindSuite) TestStorageKindValuesAligned(c *tc.C) {

	rows, err := s.DB().QueryContext(
		c.Context(),
		"SELECT id, kind FROM storage_kind",
	)
	c.Assert(err, tc.ErrorIsNil)
	defer func() { _ = rows.Close() }()

	dbValues := map[Kind]string{}
	for rows.Next() {
		var (
			id   int
			kind string
		)

		c.Assert(rows.Scan(&id, &kind), tc.ErrorIsNil)
		dbValues[Kind(id)] = kind
	}

	c.Check(dbValues, tc.DeepEquals, map[Kind]string{
		KindBlock:      "block",
		KindFilesystem: "filesystem",
	})
}
