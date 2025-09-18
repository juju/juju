// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"testing"

	"github.com/juju/tc"

	schematesting "github.com/juju/juju/domain/schema/testing"
	domainstorage "github.com/juju/juju/domain/storage"
)

type schemaAssumptionSuite struct {
	schematesting.ModelSuite
}

func TestSchemaAssumptionSuite(t *testing.T) {
	tc.Run(t, &schemaAssumptionSuite{})
}

// TestNewStoragePoolDefaultsToUser is a regression test to assert that new
// storage pools created in the model default to having their origin set to
// user.
//
// If this test fails it means the schema default as changed on the storage_pool
// table and is no longer correct. This test came about after the origin id
// values were changed and broke existing storage pool logic.
func (s *schemaAssumptionSuite) TestNewStoragePoolDefaultsToUser(c *tc.C) {
	_, err := s.DB().ExecContext(
		c.Context(),
		`
INSERT INTO storage_pool (uuid, name, type) VALUES ('uuid', 'test-pool', 'lxd')
`,
	)
	c.Assert(err, tc.ErrorIsNil)

	var originID int
	err = s.DB().QueryRowContext(
		c.Context(),
		"SELECT origin_id FROM storage_pool WHERE uuid = 'uuid'",
	).Scan(&originID)
	c.Check(err, tc.ErrorIsNil)
	c.Check(domainstorage.StoragePoolOrigin(originID), tc.Equals, domainstorage.StoragePoolOriginUser)
}
