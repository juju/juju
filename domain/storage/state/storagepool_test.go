// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"

	"github.com/juju/tc"
	jc "github.com/juju/testing/checkers"

	coredatabase "github.com/juju/juju/core/database"
	"github.com/juju/juju/domain"
	"github.com/juju/juju/domain/schema/testing"
	domainstorage "github.com/juju/juju/domain/storage"
	storageerrors "github.com/juju/juju/domain/storage/errors"
	"github.com/juju/juju/internal/errors"
)

type storagePoolSuite struct {
	testing.ModelSuite
}

var _ = tc.Suite(&storagePoolSuite{})

func newStoragePoolState(factory coredatabase.TxnRunnerFactory) *State {
	return &State{
		StateBase: domain.NewStateBase(factory),
	}
}

func (s *storagePoolSuite) TestCreateStoragePool(c *tc.C) {
	st := newStoragePoolState(s.TxnRunnerFactory())

	sp := domainstorage.StoragePoolDetails{
		Name:     "ebs-fast",
		Provider: "ebs",
		Attrs: map[string]string{
			"foo": "foo val",
			"bar": "bar val",
		},
	}
	ctx := context.Background()
	err := st.CreateStoragePool(ctx, sp)
	c.Assert(err, jc.ErrorIsNil)

	out, err := st.GetStoragePoolByName(ctx, "ebs-fast")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(out, jc.DeepEquals, sp)
}

func (s *storagePoolSuite) TestCreateStoragePoolNoAttributes(c *tc.C) {
	st := newStoragePoolState(s.TxnRunnerFactory())

	sp := domainstorage.StoragePoolDetails{
		Name:     "ebs-fast",
		Provider: "ebs",
	}
	ctx := context.Background()
	err := st.CreateStoragePool(ctx, sp)
	c.Assert(err, jc.ErrorIsNil)

	out, err := st.GetStoragePoolByName(ctx, "ebs-fast")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(out, jc.DeepEquals, sp)
}

func (s *storagePoolSuite) TestCreateStoragePoolAlreadyExists(c *tc.C) {
	st := newStoragePoolState(s.TxnRunnerFactory())

	sp := domainstorage.StoragePoolDetails{
		Name:     "ebs-fast",
		Provider: "ebs",
		Attrs: map[string]string{
			"foo": "foo val",
			"bar": "bar val",
		},
	}
	ctx := context.Background()
	err := st.CreateStoragePool(ctx, sp)
	c.Assert(err, jc.ErrorIsNil)

	err = st.CreateStoragePool(ctx, sp)
	c.Assert(err, jc.ErrorIs, storageerrors.PoolAlreadyExists)
}

func (s *storagePoolSuite) TestUpdateCloudCredentialMissingName(c *tc.C) {
	st := newStoragePoolState(s.TxnRunnerFactory())

	sp := domainstorage.StoragePoolDetails{
		Provider: "ebs",
	}
	ctx := context.Background()
	err := st.CreateStoragePool(ctx, sp)
	c.Assert(errors.Is(err, storageerrors.MissingPoolNameError), jc.IsTrue)
}

func (s *storagePoolSuite) TestUpdateCloudCredentialMissingProvider(c *tc.C) {
	st := newStoragePoolState(s.TxnRunnerFactory())

	sp := domainstorage.StoragePoolDetails{
		Name: "ebs-fast",
	}
	ctx := context.Background()
	err := st.CreateStoragePool(ctx, sp)
	c.Assert(errors.Is(err, storageerrors.MissingPoolTypeError), jc.IsTrue)
}

func (s *storagePoolSuite) TestReplaceStoragePool(c *tc.C) {
	st := newStoragePoolState(s.TxnRunnerFactory())

	sp := domainstorage.StoragePoolDetails{
		Name:     "ebs-fast",
		Provider: "ebs",
		Attrs: map[string]string{
			"foo": "foo val",
			"bar": "bar val",
		},
	}
	ctx := context.Background()
	err := st.CreateStoragePool(ctx, sp)
	c.Assert(err, jc.ErrorIsNil)

	sp2 := domainstorage.StoragePoolDetails{
		Name:     "ebs-fast",
		Provider: "ebs",
		Attrs: map[string]string{
			"baz": "baz val",
		},
	}
	err = st.ReplaceStoragePool(ctx, sp2)
	c.Assert(err, jc.ErrorIsNil)

	out, err := st.GetStoragePoolByName(ctx, "ebs-fast")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(out, jc.DeepEquals, sp2)
}

func (s *storagePoolSuite) TestReplaceStoragePoolNoAttributes(c *tc.C) {
	st := newStoragePoolState(s.TxnRunnerFactory())

	sp := domainstorage.StoragePoolDetails{
		Name:     "ebs-fast",
		Provider: "ebs",
		Attrs: map[string]string{
			"foo": "foo val",
			"bar": "bar val",
		},
	}
	ctx := context.Background()
	err := st.CreateStoragePool(ctx, sp)
	c.Assert(err, jc.ErrorIsNil)

	sp2 := domainstorage.StoragePoolDetails{
		Name:     "ebs-fast",
		Provider: "ebs",
	}
	err = st.ReplaceStoragePool(ctx, sp2)
	c.Assert(err, jc.ErrorIsNil)

	out, err := st.GetStoragePoolByName(ctx, "ebs-fast")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(out, jc.DeepEquals, sp2)
}

func (s *storagePoolSuite) TestReplaceStoragePoolNotFound(c *tc.C) {
	st := newStoragePoolState(s.TxnRunnerFactory())

	sp := domainstorage.StoragePoolDetails{
		Name:     "ebs-fast",
		Provider: "ebs",
		Attrs: map[string]string{
			"baz": "baz val",
		},
	}
	ctx := context.Background()
	err := st.ReplaceStoragePool(ctx, sp)
	c.Assert(err, jc.ErrorIs, storageerrors.PoolNotFoundError)
}

func (s *storagePoolSuite) TestDeleteStoragePool(c *tc.C) {
	st := newStoragePoolState(s.TxnRunnerFactory())

	sp := domainstorage.StoragePoolDetails{
		Name:     "ebs-fast",
		Provider: "ebs",
		Attrs: map[string]string{
			"foo": "foo val",
			"bar": "bar val",
		},
	}
	ctx := context.Background()
	err := st.CreateStoragePool(ctx, sp)
	c.Assert(err, jc.ErrorIsNil)

	err = st.DeleteStoragePool(ctx, "ebs-fast")
	c.Assert(err, jc.ErrorIsNil)

	_, err = st.GetStoragePoolByName(ctx, "ebs-fast")
	c.Assert(err, jc.ErrorIs, storageerrors.PoolNotFoundError)
}

func (s *storagePoolSuite) TestDeleteStoragePoolNotFound(c *tc.C) {
	st := newStoragePoolState(s.TxnRunnerFactory())

	ctx := context.Background()
	err := st.DeleteStoragePool(ctx, "ebs-fast")
	c.Assert(err, jc.ErrorIs, storageerrors.PoolNotFoundError)
}

func (s *storagePoolSuite) TestListStoragePools(c *tc.C) {
	st := newStoragePoolState(s.TxnRunnerFactory())

	sp := domainstorage.StoragePoolDetails{
		Name:     "ebs-fast",
		Provider: "ebs",
		Attrs: map[string]string{
			"foo": "foo val",
			"bar": "bar val",
		},
	}
	sp2 := domainstorage.StoragePoolDetails{
		Name:     "ebs-faster",
		Provider: "ebs",
		Attrs: map[string]string{
			"baz": "baz val",
		},
	}
	ctx := context.Background()
	err := st.CreateStoragePool(ctx, sp)
	c.Assert(err, jc.ErrorIsNil)
	err = st.CreateStoragePool(ctx, sp2)
	c.Assert(err, jc.ErrorIsNil)

	out, err := st.ListStoragePools(context.Background(), domainstorage.NilNames, domainstorage.NilProviders)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(out, jc.SameContents, []domainstorage.StoragePoolDetails{sp, sp2})
}

func (s *storagePoolSuite) TestStoragePoolsEmpty(c *tc.C) {
	st := newStoragePoolState(s.TxnRunnerFactory())

	creds, err := st.ListStoragePools(context.Background(), domainstorage.NilNames, domainstorage.NilProviders)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(creds, tc.HasLen, 0)
}

func (s *storagePoolSuite) TestGetStoragePoolByName(c *tc.C) {
	st := newStoragePoolState(s.TxnRunnerFactory())

	sp := domainstorage.StoragePoolDetails{
		Name:     "ebs-fast",
		Provider: "ebs",
		Attrs: map[string]string{
			"foo": "foo val",
			"bar": "bar val",
		},
	}
	sp2 := domainstorage.StoragePoolDetails{
		Name:     "loop",
		Provider: "loop",
	}
	ctx := context.Background()
	err := st.CreateStoragePool(ctx, sp)
	c.Assert(err, jc.ErrorIsNil)
	err = st.CreateStoragePool(ctx, sp2)
	c.Assert(err, jc.ErrorIsNil)

	out, err := st.GetStoragePoolByName(context.Background(), "ebs-fast")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(out, jc.DeepEquals, sp)
}

func (s *storagePoolSuite) TestListStoragePoolsFilterOnNameAndProvider(c *tc.C) {
	st := newStoragePoolState(s.TxnRunnerFactory())

	sp := domainstorage.StoragePoolDetails{
		Name:     "ebs-fast",
		Provider: "ebs",
		Attrs: map[string]string{
			"foo": "foo val",
			"bar": "bar val",
		},
	}
	sp2 := domainstorage.StoragePoolDetails{
		Name:     "loop",
		Provider: "loop",
	}
	ctx := context.Background()
	err := st.CreateStoragePool(ctx, sp)
	c.Assert(err, jc.ErrorIsNil)
	err = st.CreateStoragePool(ctx, sp2)
	c.Assert(err, jc.ErrorIsNil)

	out, err := st.ListStoragePools(context.Background(), domainstorage.Names{"ebs-fast"}, domainstorage.Providers{"ebs"})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(out, jc.SameContents, []domainstorage.StoragePoolDetails{sp})
}

func (s *storagePoolSuite) TestListStoragePoolsFilterOnName(c *tc.C) {
	st := newStoragePoolState(s.TxnRunnerFactory())

	sp := domainstorage.StoragePoolDetails{
		Name:     "ebs-fast",
		Provider: "ebs",
		Attrs: map[string]string{
			"foo": "foo val",
			"bar": "bar val",
		},
	}
	sp2 := domainstorage.StoragePoolDetails{
		Name:     "loop",
		Provider: "loop",
	}
	ctx := context.Background()
	err := st.CreateStoragePool(ctx, sp)
	c.Assert(err, jc.ErrorIsNil)
	err = st.CreateStoragePool(ctx, sp2)
	c.Assert(err, jc.ErrorIsNil)

	out, err := st.ListStoragePools(context.Background(), domainstorage.Names{"loop"}, domainstorage.NilProviders)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(out, jc.SameContents, []domainstorage.StoragePoolDetails{sp2})
}

func (s *storagePoolSuite) TestListStoragePoolsFilterOnProvider(c *tc.C) {
	st := newStoragePoolState(s.TxnRunnerFactory())

	sp := domainstorage.StoragePoolDetails{
		Name:     "ebs-fast",
		Provider: "ebs",
		Attrs: map[string]string{
			"foo": "foo val",
			"bar": "bar val",
		},
	}
	sp2 := domainstorage.StoragePoolDetails{
		Name:     "loop",
		Provider: "loop",
	}
	ctx := context.Background()
	err := st.CreateStoragePool(ctx, sp)
	c.Assert(err, jc.ErrorIsNil)
	err = st.CreateStoragePool(ctx, sp2)
	c.Assert(err, jc.ErrorIsNil)

	out, err := st.ListStoragePools(context.Background(), domainstorage.NilNames, domainstorage.Providers{"ebs"})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(out, jc.SameContents, []domainstorage.StoragePoolDetails{sp})
}
