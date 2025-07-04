// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"database/sql"
	stdtesting "testing"

	"github.com/canonical/sqlair"
	"github.com/juju/tc"

	"github.com/juju/juju/domain/schema/testing"
	domainstorage "github.com/juju/juju/domain/storage"
	storageerrors "github.com/juju/juju/domain/storage/errors"
)

type storagePoolHelperSuite struct {
	testing.ModelSuite
}

func TestStoragePoolHelperSuite(t *stdtesting.T) {
	tc.Run(t, &storagePoolHelperSuite{})
}

func (s *storagePoolHelperSuite) TestGetStoragePoolUUID(c *tc.C) {
	st := newStoragePoolState(s.TxnRunnerFactory())

	sp := domainstorage.StoragePool{
		Name:     "ebs-fast",
		Provider: "ebs",
		Attrs: map[string]string{
			"foo": "foo val",
			"bar": "bar val",
		},
	}

	ctx := c.Context()
	err := st.CreateStoragePool(ctx, sp)
	c.Assert(err, tc.ErrorIsNil)

	var poolUUIDStr string
	err = s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		return tx.QueryRowContext(ctx, `
SELECT sp.uuid
FROM   storage_pool sp
WHERE  sp.name = ?`, "ebs-fast").Scan(&poolUUIDStr)
	})
	c.Assert(err, tc.ErrorIsNil)

	db, err := st.DB()
	c.Assert(err, tc.ErrorIsNil)

	var poolUUID domainstorage.StoragePoolUUID
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		poolUUID, err = GetStoragePoolUUID(ctx, tx, st, "ebs-fast")
		return err
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(poolUUID.String(), tc.Equals, poolUUIDStr)
}

func (s *storagePoolHelperSuite) TestGetStoragePoolUUIDNotFound(c *tc.C) {
	st := newStoragePoolState(s.TxnRunnerFactory())

	db, err := st.DB()
	c.Assert(err, tc.ErrorIsNil)

	err = db.Txn(c.Context(), func(ctx context.Context, tx *sqlair.TX) error {
		_, err := GetStoragePoolUUID(ctx, tx, st, "non-existent-pool")
		return err
	})
	c.Assert(err, tc.ErrorIs, storageerrors.PoolNotFoundError)
}

func (s *storagePoolHelperSuite) TestGetStoragePool(c *tc.C) {
	st := newStoragePoolState(s.TxnRunnerFactory())

	sp := domainstorage.StoragePool{
		Name:     "ebs-fast",
		Provider: "ebs",
		Attrs: map[string]string{
			"foo": "foo val",
			"bar": "bar val",
		},
	}

	ctx := c.Context()
	err := st.CreateStoragePool(ctx, sp)
	c.Assert(err, tc.ErrorIsNil)

	poolUUID, err := st.GetStoragePoolUUID(ctx, "ebs-fast")
	c.Assert(err, tc.ErrorIsNil)

	db, err := st.DB()
	c.Assert(err, tc.ErrorIsNil)

	var pool domainstorage.StoragePool
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		pool, err = GetStoragePool(ctx, tx, st, poolUUID)
		return err
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(pool, tc.DeepEquals, sp)
}

func (s *storagePoolHelperSuite) TestGetStoragePoolNotFound(c *tc.C) {
	st := newStoragePoolState(s.TxnRunnerFactory())

	poolUUID, err := domainstorage.NewStoragePoolUUID()
	c.Assert(err, tc.ErrorIsNil)

	db, err := st.DB()
	c.Assert(err, tc.ErrorIsNil)

	err = db.Txn(c.Context(), func(ctx context.Context, tx *sqlair.TX) error {
		_, err := GetStoragePool(ctx, tx, st, poolUUID)
		return err
	})
	c.Assert(err, tc.ErrorIs, storageerrors.PoolNotFoundError)
}
