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
	// Ensure we use one time for all the tests, so that we can reliably compare
	// the time without worrying about the time changing from one test to
	// another, causing intermittent failure.
	now = time.Now()

	key0 = macaroon.RootKey{
		ID:      []byte("0"),
		Created: now,
		Expires: now.Add(2 * time.Second),
		RootKey: []byte("key0"),
	}

	key1 = macaroon.RootKey{
		ID:      []byte("1"),
		Created: now.Add(2 * time.Second),
		Expires: now.Add(4 * time.Second),
		RootKey: []byte("key1"),
	}

	key2 = macaroon.RootKey{
		ID:      []byte("2"),
		Created: now.Add(4 * time.Second),
		Expires: now.Add(8 * time.Second),
		RootKey: []byte("key2"),
	}

	key3 = macaroon.RootKey{
		ID:      []byte("3"),
		Created: now.Add(6 * time.Second),
		Expires: now.Add(6 * time.Second),
		RootKey: []byte("key3"),
	}

	key4 = macaroon.RootKey{
		ID:      []byte("4"),
		Created: now.Add(-time.Second),
		Expires: now.Add(-time.Second),
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

	res, err := st.FindLatestKey(ctx, time.Unix(0, 0), now.Add(7*time.Second), time.Unix(math.MaxInt64, 0))
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

	res, err := st.FindLatestKey(ctx, time.Unix(0, 0), time.Unix(0, 0), now.Add(5*time.Second))
	c.Assert(err, jc.ErrorIsNil)
	compareRootKeys(c, key1, res)

	res, err = st.FindLatestKey(ctx, time.Unix(0, 0), time.Unix(0, 0), now.Add(3*time.Second))
	c.Assert(err, jc.ErrorIsNil)
	compareRootKeys(c, key0, res)

	_, err = st.FindLatestKey(ctx, time.Unix(0, 0), time.Unix(0, 0), now.Add(-2*time.Second))
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

func (s *rootKeyStateSuite) TestRemoveKeysExpiredBeforeNow(c *gc.C) {
	st := NewState(s.TxnRunnerFactory())
	ctx := context.Background()
	addAllKeys(c, st)

	err := st.RemoveKeysExpiredBefore(ctx, now)
	c.Assert(err, jc.ErrorIsNil)

	_, err = st.GetKey(ctx, key0.ID)
	c.Check(err, jc.ErrorIsNil)
	_, err = st.GetKey(ctx, key1.ID)
	c.Check(err, jc.ErrorIsNil)
	_, err = st.GetKey(ctx, key2.ID)
	c.Check(err, jc.ErrorIsNil)
	_, err = st.GetKey(ctx, key3.ID)
	c.Check(err, jc.ErrorIsNil)
	_, err = st.GetKey(ctx, key4.ID)
	c.Check(err, jc.ErrorIs, errors.KeyNotFound)
}

func (s *rootKeyStateSuite) TestRemoveKeysExpiredBeforeNowPlus5(c *gc.C) {
	st := NewState(s.TxnRunnerFactory())
	ctx := context.Background()
	addAllKeys(c, st)

	err := st.RemoveKeysExpiredBefore(ctx, now.Add(5*time.Second))
	c.Assert(err, jc.ErrorIsNil)

	_, err = st.GetKey(ctx, key0.ID)
	c.Check(err, jc.ErrorIs, errors.KeyNotFound)
	_, err = st.GetKey(ctx, key1.ID)
	c.Check(err, jc.ErrorIs, errors.KeyNotFound)
	_, err = st.GetKey(ctx, key2.ID)
	c.Check(err, jc.ErrorIsNil)
	_, err = st.GetKey(ctx, key3.ID)
	c.Check(err, jc.ErrorIsNil)
	_, err = st.GetKey(ctx, key4.ID)
	c.Check(err, jc.ErrorIs, errors.KeyNotFound)
}

func (s *rootKeyStateSuite) TestRemoveKeysExpiredAll(c *gc.C) {
	st := NewState(s.TxnRunnerFactory())
	ctx := context.Background()
	addAllKeys(c, st)

	err := st.RemoveKeysExpiredBefore(ctx, now.Add(10*time.Second))
	c.Assert(err, jc.ErrorIsNil)

	_, err = st.GetKey(ctx, key0.ID)
	c.Check(err, jc.ErrorIs, errors.KeyNotFound)
	_, err = st.GetKey(ctx, key1.ID)
	c.Check(err, jc.ErrorIs, errors.KeyNotFound)
	_, err = st.GetKey(ctx, key2.ID)
	c.Check(err, jc.ErrorIs, errors.KeyNotFound)
	_, err = st.GetKey(ctx, key3.ID)
	c.Check(err, jc.ErrorIs, errors.KeyNotFound)
	_, err = st.GetKey(ctx, key4.ID)
	c.Check(err, jc.ErrorIs, errors.KeyNotFound)

	_, err = st.FindLatestKey(ctx, time.Unix(0, 0), time.Unix(0, 0), time.Unix(math.MaxInt64, 0))
	c.Check(err, jc.ErrorIs, errors.KeyNotFound)
}

func (s *rootKeyStateSuite) TestRemoveKeysExpiredNone(c *gc.C) {
	st := NewState(s.TxnRunnerFactory())
	ctx := context.Background()
	addAllKeys(c, st)

	err := st.RemoveKeysExpiredBefore(ctx, now.Add(-10*time.Second))
	c.Assert(err, jc.ErrorIsNil)

	_, err = st.GetKey(ctx, key0.ID)
	c.Check(err, jc.ErrorIsNil)
	_, err = st.GetKey(ctx, key1.ID)
	c.Check(err, jc.ErrorIsNil)
	_, err = st.GetKey(ctx, key2.ID)
	c.Check(err, jc.ErrorIsNil)
	_, err = st.GetKey(ctx, key3.ID)
	c.Check(err, jc.ErrorIsNil)
	_, err = st.GetKey(ctx, key4.ID)
	c.Check(err, jc.ErrorIsNil)
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
	c.Check(k1.ID, gc.DeepEquals, k2.ID)
	c.Check(k1.RootKey, gc.DeepEquals, k2.RootKey)
	c.Check(k1.Created.Compare(k2.Created), gc.Equals, 0)
	c.Check(k1.Expires.Compare(k2.Expires), gc.Equals, 0)
}
