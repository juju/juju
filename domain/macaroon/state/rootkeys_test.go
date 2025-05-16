// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"math"
	stdtesting "testing"
	"time"

	"github.com/juju/tc"

	"github.com/juju/juju/domain/macaroon"
	"github.com/juju/juju/domain/macaroon/errors"
	"github.com/juju/juju/internal/changestream/testing"
)

type rootKeyStateSuite struct {
	testing.ControllerSuite
}

func TestRootKeyStateSuite(t *stdtesting.T) { tc.Run(t, &rootKeyStateSuite{}) }

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

func (s *rootKeyStateSuite) TestInsertAndGetKey(c *tc.C) {
	st := NewState(s.TxnRunnerFactory())
	ctx := c.Context()

	_, err := st.GetKey(ctx, key0.ID, now)
	c.Assert(err, tc.ErrorIs, errors.KeyNotFound)

	err = st.InsertKey(ctx, key0)
	c.Assert(err, tc.ErrorIsNil)

	res, err := st.GetKey(ctx, key0.ID, now)
	c.Assert(err, tc.ErrorIsNil)
	compareRootKeys(c, key0, res)
}

func (s *rootKeyStateSuite) TestInsertKeyIDUniqueness(c *tc.C) {
	st := NewState(s.TxnRunnerFactory())
	ctx := c.Context()

	_, err := st.GetKey(ctx, key0.ID, now)
	c.Assert(err, tc.ErrorIs, errors.KeyNotFound)

	err = st.InsertKey(ctx, key0)
	c.Assert(err, tc.ErrorIsNil)

	err = st.InsertKey(ctx, key0)
	c.Assert(err, tc.ErrorIs, errors.KeyAlreadyExists)
}

func (s *rootKeyStateSuite) TestFindLatestKeyReturnsMostRecent(c *tc.C) {
	st := NewState(s.TxnRunnerFactory())
	ctx := c.Context()
	addAllKeys(c, st)

	res, err := st.FindLatestKey(ctx, time.Unix(0, 0), time.Unix(0, 0), time.Unix(math.MaxInt64, 0), now)
	c.Assert(err, tc.ErrorIsNil)
	compareRootKeys(c, key3, res)
}

func (s *rootKeyStateSuite) TestFindLatestKeyExpiresAfter(c *tc.C) {
	st := NewState(s.TxnRunnerFactory())
	ctx := c.Context()
	addAllKeys(c, st)

	res, err := st.FindLatestKey(ctx, time.Unix(0, 0), now.Add(7*time.Second), time.Unix(math.MaxInt64, 0), now)
	c.Assert(err, tc.ErrorIsNil)
	compareRootKeys(c, key2, res)
}

func (s *rootKeyStateSuite) TestFindLatestKeyCreatedAfter(c *tc.C) {
	st := NewState(s.TxnRunnerFactory())
	ctx := c.Context()
	addAllKeys(c, st)

	res, err := st.FindLatestKey(ctx, time.Unix(0, 0), time.Unix(0, 0), time.Unix(math.MaxInt64, 0), now)
	c.Assert(err, tc.ErrorIsNil)
	compareRootKeys(c, key3, res)

	_, err = st.FindLatestKey(ctx, time.Unix(math.MaxInt64, 0), time.Unix(0, 0), time.Unix(math.MaxInt64, 0), now)
	c.Assert(err, tc.ErrorIs, errors.KeyNotFound)
}

func (s *rootKeyStateSuite) TestFindLatestKeyExpiresBefore(c *tc.C) {
	st := NewState(s.TxnRunnerFactory())
	ctx := c.Context()
	addAllKeys(c, st)

	res, err := st.FindLatestKey(ctx, time.Unix(0, 0), time.Unix(0, 0), now.Add(5*time.Second), now)
	c.Assert(err, tc.ErrorIsNil)
	compareRootKeys(c, key1, res)

	res, err = st.FindLatestKey(ctx, time.Unix(0, 0), time.Unix(0, 0), now.Add(3*time.Second), now)
	c.Assert(err, tc.ErrorIsNil)
	compareRootKeys(c, key0, res)

	_, err = st.FindLatestKey(ctx, time.Unix(0, 0), time.Unix(0, 0), now.Add(-2*time.Second), now)
	c.Assert(err, tc.ErrorIs, errors.KeyNotFound)
}

func (s *rootKeyStateSuite) TestFindLatestKeyEquality(c *tc.C) {
	st := NewState(s.TxnRunnerFactory())
	ctx := c.Context()
	addAllKeys(c, st)

	res, err := st.FindLatestKey(ctx, key3.Created, key3.Expires, key3.Expires, now)
	c.Assert(err, tc.ErrorIsNil)
	compareRootKeys(c, key3, res)

	_, err = st.FindLatestKey(ctx, key3.Created.Add(1*time.Millisecond), key3.Expires, key3.Expires, now)
	c.Assert(err, tc.ErrorIs, errors.KeyNotFound)
}

func (s *rootKeyStateSuite) TestExpiredKeysAreRemoved(c *tc.C) {
	st := NewState(s.TxnRunnerFactory())
	ctx := c.Context()
	addAllKeys(c, st)

	_, err := st.GetKey(ctx, key0.ID, now)
	c.Check(err, tc.ErrorIsNil)
	_, err = st.GetKey(ctx, key1.ID, now)
	c.Check(err, tc.ErrorIsNil)
	_, err = st.GetKey(ctx, key2.ID, now)
	c.Check(err, tc.ErrorIsNil)
	_, err = st.GetKey(ctx, key3.ID, now)
	c.Check(err, tc.ErrorIsNil)
	_, err = st.GetKey(ctx, key4.ID, now)
	c.Check(err, tc.ErrorIs, errors.KeyNotFound)
}

func (s *rootKeyStateSuite) TestRemoveKeysExpiredBeforeNowPlus5(c *tc.C) {
	st := NewState(s.TxnRunnerFactory())
	ctx := c.Context()
	addAllKeys(c, st)

	now := now.Add(5 * time.Second)

	_, err := st.GetKey(ctx, key0.ID, now)
	c.Check(err, tc.ErrorIs, errors.KeyNotFound)
	_, err = st.GetKey(ctx, key1.ID, now)
	c.Check(err, tc.ErrorIs, errors.KeyNotFound)
	_, err = st.GetKey(ctx, key2.ID, now)
	c.Check(err, tc.ErrorIsNil)
	_, err = st.GetKey(ctx, key3.ID, now)
	c.Check(err, tc.ErrorIsNil)
	_, err = st.GetKey(ctx, key4.ID, now)
	c.Check(err, tc.ErrorIs, errors.KeyNotFound)
}

func (s *rootKeyStateSuite) TestRemoveKeysExpiredAll(c *tc.C) {
	st := NewState(s.TxnRunnerFactory())
	ctx := c.Context()
	addAllKeys(c, st)

	now := now.Add(10 * time.Second)

	_, err := st.GetKey(ctx, key0.ID, now)
	c.Check(err, tc.ErrorIs, errors.KeyNotFound)
	_, err = st.GetKey(ctx, key1.ID, now)
	c.Check(err, tc.ErrorIs, errors.KeyNotFound)
	_, err = st.GetKey(ctx, key2.ID, now)
	c.Check(err, tc.ErrorIs, errors.KeyNotFound)
	_, err = st.GetKey(ctx, key3.ID, now)
	c.Check(err, tc.ErrorIs, errors.KeyNotFound)
	_, err = st.GetKey(ctx, key4.ID, now)
	c.Check(err, tc.ErrorIs, errors.KeyNotFound)

	_, err = st.FindLatestKey(ctx, time.Unix(0, 0), time.Unix(0, 0), time.Unix(math.MaxInt64, 0), now)
	c.Check(err, tc.ErrorIs, errors.KeyNotFound)
}

func (s *rootKeyStateSuite) TestRemoveKeysExpiredNone(c *tc.C) {
	st := NewState(s.TxnRunnerFactory())
	ctx := c.Context()
	addAllKeys(c, st)

	now := now.Add(-10 * time.Second)

	_, err := st.GetKey(ctx, key0.ID, now)
	c.Check(err, tc.ErrorIsNil)
	_, err = st.GetKey(ctx, key1.ID, now)
	c.Check(err, tc.ErrorIsNil)
	_, err = st.GetKey(ctx, key2.ID, now)
	c.Check(err, tc.ErrorIsNil)
	_, err = st.GetKey(ctx, key3.ID, now)
	c.Check(err, tc.ErrorIsNil)
	_, err = st.GetKey(ctx, key4.ID, now)
	c.Check(err, tc.ErrorIsNil)
}

func addAllKeys(c *tc.C, st *State) {
	ctx := c.Context()

	err := st.InsertKey(ctx, key0)
	c.Assert(err, tc.ErrorIsNil)
	err = st.InsertKey(ctx, key1)
	c.Assert(err, tc.ErrorIsNil)
	err = st.InsertKey(ctx, key2)
	c.Assert(err, tc.ErrorIsNil)
	err = st.InsertKey(ctx, key3)
	c.Assert(err, tc.ErrorIsNil)
	err = st.InsertKey(ctx, key4)
	c.Assert(err, tc.ErrorIsNil)
}

func compareRootKeys(c *tc.C, k1, k2 macaroon.RootKey) {
	c.Check(k1.ID, tc.DeepEquals, k2.ID)
	c.Check(k1.RootKey, tc.DeepEquals, k2.RootKey)
	c.Check(k1.Created.Compare(k2.Created), tc.Equals, 0)
	c.Check(k1.Expires.Compare(k2.Expires), tc.Equals, 0)
}
