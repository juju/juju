// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"math"
	"time"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/domain/macaroon"
	"github.com/juju/juju/domain/macaroon/errors"
	"github.com/juju/juju/internal/changestream/testing"
)

type rootKeyStateSuite struct {
	testing.ControllerSuite
}

var _ = gc.Suite(&rootKeyStateSuite{})

var (
	key0 = macaroon.RootKey{
		ID:      []byte("0"),
		Created: time.Now(),
		Expires: time.Now().Add(2 * time.Second),
		RootKey: []byte("key0"),
	}

	key1 = macaroon.RootKey{
		ID:      []byte("1"),
		Created: time.Now().Add(2 * time.Second),
		Expires: time.Now().Add(4 * time.Second),
		RootKey: []byte("key1"),
	}

	key2 = macaroon.RootKey{
		ID:      []byte("2"),
		Created: time.Now().Add(4 * time.Second),
		Expires: time.Now().Add(8 * time.Second),
		RootKey: []byte("key2"),
	}

	key3 = macaroon.RootKey{
		ID:      []byte("3"),
		Created: time.Now().Add(6 * time.Second),
		Expires: time.Now().Add(6 * time.Second),
		RootKey: []byte("key3"),
	}

	key4 = macaroon.RootKey{
		ID:      []byte("4"),
		Created: time.Now().Add(-time.Second),
		Expires: time.Now().Add(-time.Second),
		RootKey: []byte("key4"),
	}
)

func (s *rootKeyStateSuite) TestInsertAndGetKey(c *gc.C) {
	st := NewState(s.TxnRunnerFactory())
	ctx := context.Background()

	_, err := st.GetKey(ctx, key0.ID)
	c.Assert(err, jc.ErrorIs, errors.KeyNotFound)

	err = st.InsertKey(ctx, key0)
	c.Assert(err, jc.ErrorIsNil)

	res, err := st.GetKey(context.Background(), key0.ID)
	c.Assert(err, jc.ErrorIsNil)
	compareRootKeys(c, key0, res)
}

func (s *rootKeyStateSuite) TestInsertKeyIDUniqueness(c *gc.C) {
	st := NewState(s.TxnRunnerFactory())
	ctx := context.Background()

	_, err := st.GetKey(ctx, key0.ID)
	c.Assert(err, jc.ErrorIs, errors.KeyNotFound)

	err = st.InsertKey(ctx, key0)
	c.Assert(err, jc.ErrorIsNil)

	err = st.InsertKey(ctx, key0)
	c.Assert(err, jc.ErrorIs, errors.KeyAlreadyExists)
}

func (s *rootKeyStateSuite) TestFindLatestKeyReturnsMostRecent(c *gc.C) {
	st := NewState(s.TxnRunnerFactory())
	ctx := context.Background()
	addAllKeys(c, st)

	res, err := st.FindLatestKey(ctx, time.Unix(0, 0), time.Unix(0, 0), time.Unix(math.MaxInt64, 0))
	c.Assert(err, jc.ErrorIsNil)
	compareRootKeys(c, key3, res)
}

func (s *rootKeyStateSuite) TestFindLatestKeyExpiresAfter(c *gc.C) {
	st := NewState(s.TxnRunnerFactory())
	ctx := context.Background()
	addAllKeys(c, st)

	res, err := st.FindLatestKey(ctx, time.Unix(0, 0), time.Now().Add(7*time.Second), time.Unix(math.MaxInt64, 0))
	c.Assert(err, jc.ErrorIsNil)
	compareRootKeys(c, key2, res)
}

func (s *rootKeyStateSuite) TestFindLatestKeyCreatedAfter(c *gc.C) {
	st := NewState(s.TxnRunnerFactory())
	ctx := context.Background()
	addAllKeys(c, st)

	res, err := st.FindLatestKey(ctx, time.Unix(0, 0), time.Unix(0, 0), time.Unix(math.MaxInt64, 0))
	c.Assert(err, jc.ErrorIsNil)
	compareRootKeys(c, key3, res)

	_, err = st.FindLatestKey(ctx, time.Unix(math.MaxInt64, 0), time.Unix(0, 0), time.Unix(math.MaxInt64, 0))
	c.Assert(err, jc.ErrorIs, errors.KeyNotFound)
}

func (s *rootKeyStateSuite) TestFindLatestKeyExpiresBefore(c *gc.C) {
	st := NewState(s.TxnRunnerFactory())
	ctx := context.Background()
	addAllKeys(c, st)

	res, err := st.FindLatestKey(ctx, time.Unix(0, 0), time.Unix(0, 0), time.Now().Add(5*time.Second))
	c.Assert(err, jc.ErrorIsNil)
	compareRootKeys(c, key1, res)

	res, err = st.FindLatestKey(ctx, time.Unix(0, 0), time.Unix(0, 0), time.Now().Add(3*time.Second))
	c.Assert(err, jc.ErrorIsNil)
	compareRootKeys(c, key0, res)

	_, err = st.FindLatestKey(ctx, time.Unix(0, 0), time.Unix(0, 0), time.Now().Add(-2*time.Second))
	c.Assert(err, jc.ErrorIs, errors.KeyNotFound)
}

func (s *rootKeyStateSuite) TestFindLatestKeyEquality(c *gc.C) {
	st := NewState(s.TxnRunnerFactory())
	ctx := context.Background()
	addAllKeys(c, st)

	res, err := st.FindLatestKey(ctx, key3.Created, key3.Expires, key3.Expires)
	c.Assert(err, jc.ErrorIsNil)
	compareRootKeys(c, key3, res)

	_, err = st.FindLatestKey(ctx, key3.Created.Add(1*time.Millisecond), key3.Expires, key3.Expires)
	c.Assert(err, jc.ErrorIs, errors.KeyNotFound)
}

func addAllKeys(c *gc.C, st *State) {
	ctx := context.Background()

	err := st.InsertKey(ctx, key0)
	c.Assert(err, jc.ErrorIsNil)
	err = st.InsertKey(ctx, key1)
	c.Assert(err, jc.ErrorIsNil)
	err = st.InsertKey(ctx, key2)
	c.Assert(err, jc.ErrorIsNil)
	err = st.InsertKey(ctx, key3)
	c.Assert(err, jc.ErrorIsNil)
	err = st.InsertKey(ctx, key4)
	c.Assert(err, jc.ErrorIsNil)
}

func compareRootKeys(c *gc.C, k1, k2 macaroon.RootKey) {
	c.Assert(k1.ID, gc.DeepEquals, k2.ID)
	c.Assert(k1.RootKey, gc.DeepEquals, k2.RootKey)
	c.Assert(k1.Created.Compare(k2.Created), gc.Equals, 0)
	c.Assert(k1.Expires.Compare(k2.Expires), gc.Equals, 0)
}
