// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"testing"

	"github.com/juju/tc"

	schematesting "github.com/juju/juju/domain/schema/testing"
	domainstorage "github.com/juju/juju/domain/storage"
	domainstorageinternal "github.com/juju/juju/domain/storage/internal"
	internaldatabase "github.com/juju/juju/internal/database"
)

// storagePoolSchemaSuite implements a set of tests for asserting the DDL schema
// around storage pools.
type storagePoolSchemaSuite struct {
	schematesting.ModelSuite
}

// TestStoragePoolSchemaSuite runs the tests contained within
// [storagePoolSchemaSuite].
func TestStoragePoolSchemaSuite(t *testing.T) {
	tc.Run(t, &storagePoolSchemaSuite{})
}

// TestStoragePoolImmutableOrigin tests that the origin of a storage pool cannot
// be changed after it has been created.
func (s *storagePoolSchemaSuite) TestStoragePoolImmutableOrigin(c *tc.C) {
	st := NewState(s.TxnRunnerFactory())

	storagePoolUUID := tc.Must(c, domainstorage.NewStoragePoolUUID)
	args := domainstorageinternal.CreateStoragePool{
		Attrs:        nil,
		Name:         "ebs-fast",
		Origin:       domainstorage.StoragePoolOriginUser,
		ProviderType: domainstorage.ProviderType("ebs"),
		UUID:         storagePoolUUID,
	}

	ctx := c.Context()
	err := st.CreateStoragePool(ctx, args)
	c.Assert(err, tc.ErrorIsNil)

	_, err = s.DB().ExecContext(
		c.Context(),
		`
UPDATE storage_pool
SET    origin_id = 2
WHERE  uuid = ?
`,
		storagePoolUUID.String(),
	)

	c.Check(internaldatabase.IsErrConstraintTrigger(err), tc.IsTrue)
}
