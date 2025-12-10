// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	stdtesting "testing"

	"github.com/canonical/sqlair"
	"github.com/juju/tc"

	"github.com/juju/juju/domain/schema/testing"
	domainstorage "github.com/juju/juju/domain/storage"
	domainstorageerrors "github.com/juju/juju/domain/storage/errors"
	domainstorageinternal "github.com/juju/juju/domain/storage/internal"
)

type storagePoolHelperSuite struct {
	testing.ModelSuite
}

func TestStoragePoolHelperSuite(t *stdtesting.T) {
	tc.Run(t, &storagePoolHelperSuite{})
}

func (s *storagePoolHelperSuite) TestGetStoragePoolUUID(c *tc.C) {
	st := NewState(s.TxnRunnerFactory())

	storagePoolUUID := tc.Must(c, domainstorage.NewStoragePoolUUID)
	sp := domainstorageinternal.CreateStoragePool{
		Attrs: map[string]string{
			"foo": "foo val",
			"bar": "bar val",
		},
		Name:         "ebs-fast",
		ProviderType: domainstorage.ProviderType("ebs"),
		UUID:         storagePoolUUID,
	}

	ctx := c.Context()
	err := st.CreateStoragePool(ctx, sp)
	c.Assert(err, tc.ErrorIsNil)

	db, err := st.DB(ctx)
	c.Assert(err, tc.ErrorIsNil)

	var poolUUID domainstorage.StoragePoolUUID
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		poolUUID, err = GetStoragePoolUUID(ctx, tx, st, "ebs-fast")
		return err
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(poolUUID.String(), tc.Equals, storagePoolUUID.String())
}

func (s *storagePoolHelperSuite) TestGetStoragePoolUUIDNotFound(c *tc.C) {
	st := NewState(s.TxnRunnerFactory())

	db, err := st.DB(c.Context())
	c.Assert(err, tc.ErrorIsNil)

	err = db.Txn(c.Context(), func(ctx context.Context, tx *sqlair.TX) error {
		_, err := GetStoragePoolUUID(ctx, tx, st, "non-existent-pool")
		return err
	})
	c.Assert(err, tc.ErrorIs, domainstorageerrors.StoragePoolNotFound)
}

func (s *storagePoolHelperSuite) TestGetStoragePool(c *tc.C) {
	st := NewState(s.TxnRunnerFactory())

	storagePoolUUID := tc.Must(c, domainstorage.NewStoragePoolUUID)
	sp := domainstorageinternal.CreateStoragePool{
		Attrs: map[string]string{
			"foo": "foo val",
			"bar": "bar val",
		},
		Name:         "ebs-fast",
		ProviderType: domainstorage.ProviderType("ebs"),
		UUID:         storagePoolUUID,
	}

	ctx := c.Context()
	err := st.CreateStoragePool(ctx, sp)
	c.Assert(err, tc.ErrorIsNil)

	pool, err := st.GetStoragePool(ctx, storagePoolUUID)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(pool, tc.DeepEquals, domainstorage.StoragePool{
		UUID:     storagePoolUUID.String(),
		Name:     "ebs-fast",
		Provider: "ebs",
		Attrs: map[string]string{
			"foo": "foo val",
			"bar": "bar val",
		},
	})
}

func (s *storagePoolHelperSuite) TestGetStoragePoolNotFound(c *tc.C) {
	st := NewState(s.TxnRunnerFactory())
	poolUUID := tc.Must(c, domainstorage.NewStoragePoolUUID)
	_, err := st.GetStoragePool(c.Context(), poolUUID)
	c.Check(err, tc.ErrorIs, domainstorageerrors.StoragePoolNotFound)
}
