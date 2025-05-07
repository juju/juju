// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"
	"time"

	"github.com/go-macaroon-bakery/macaroon-bakery/v3/bakery"
	"github.com/go-macaroon-bakery/macaroon-bakery/v3/bakery/dbrootkeystore"
	"github.com/juju/tc"
	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/domain/macaroon"
	macaroonerrors "github.com/juju/juju/domain/macaroon/errors"
	"github.com/juju/juju/internal/errors"
)

var _ dbrootkeystore.ContextBacking = &RootKeyService{}

type rootKeyServiceSuite struct {
	st    *MockState
	now   time.Time
	clock macaroon.Clock
}

var _ = tc.Suite(&rootKeyServiceSuite{})

var moment = time.Now()

var key = dbrootkeystore.RootKey{
	Id:      []byte("0"),
	Created: moment,
	Expires: moment.Add(2 * time.Second),
	RootKey: []byte("key0"),
}

func (s *rootKeyServiceSuite) SetUpTest(c *tc.C) {
	s.now = moment
	s.clock = clockVal(&s.now)
}

func (s *rootKeyServiceSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.st = NewMockState(ctrl)
	return ctrl
}

func (s *rootKeyServiceSuite) TestGetKeyContext(c *tc.C) {
	defer s.setupMocks(c).Finish()

	id := []byte("0")
	s.st.EXPECT().GetKey(gomock.Any(), id, s.now).Return(encodeRootKey(key), nil)
	srv := NewRootKeyService(s.st, s.clock)

	res, err := srv.GetKeyContext(context.Background(), id)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(res, tc.DeepEquals, key)
}

func (s *rootKeyServiceSuite) TestGetKeyContextNotFound(c *tc.C) {
	defer s.setupMocks(c).Finish()

	id := []byte("0")
	s.st.EXPECT().GetKey(gomock.Any(), id, s.now).Return(macaroon.RootKey{}, macaroonerrors.KeyNotFound)
	srv := NewRootKeyService(s.st, s.clock)

	_, err := srv.GetKeyContext(context.Background(), id)
	c.Assert(err, jc.ErrorIs, bakery.ErrNotFound)
}

func (s *rootKeyServiceSuite) TestFindLatestKeyContext(c *tc.C) {
	defer s.setupMocks(c).Finish()

	createdAfter := s.now
	expiresAfter := s.now.Add(-time.Second)
	expiresBefore := s.now.Add(time.Second)
	s.st.EXPECT().FindLatestKey(gomock.Any(), createdAfter, expiresAfter, expiresBefore, s.now).Return(encodeRootKey(key), nil)
	srv := NewRootKeyService(s.st, s.clock)

	res, err := srv.FindLatestKeyContext(context.Background(), createdAfter, expiresAfter, expiresBefore)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(res, tc.DeepEquals, key)
}

func (s *rootKeyServiceSuite) TestFindLatestKeyContextNotFound(c *tc.C) {
	defer s.setupMocks(c).Finish()

	createdAfter := s.now
	expiresAfter := s.now.Add(-time.Second)
	expiresBefore := s.now.Add(time.Second)
	s.st.EXPECT().FindLatestKey(gomock.Any(), createdAfter, expiresAfter, expiresBefore, s.now).Return(macaroon.RootKey{}, macaroonerrors.KeyNotFound)
	srv := NewRootKeyService(s.st, s.clock)

	res, err := srv.FindLatestKeyContext(context.Background(), createdAfter, expiresAfter, expiresBefore)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(res, tc.DeepEquals, dbrootkeystore.RootKey{})
}

func (s *rootKeyServiceSuite) TestInsertKeyContext(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.st.EXPECT().InsertKey(gomock.Any(), encodeRootKey(key))
	srv := NewRootKeyService(s.st, s.clock)

	err := srv.InsertKeyContext(context.Background(), key)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *rootKeyServiceSuite) TestInsertKeyContextError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	boom := errors.Errorf("boom")
	s.st.EXPECT().InsertKey(gomock.Any(), encodeRootKey(key)).Return(boom)
	srv := NewRootKeyService(s.st, s.clock)

	err := srv.InsertKeyContext(context.Background(), key)
	c.Assert(err, tc.Equals, boom)
}

func clockVal(t *time.Time) macaroon.Clock {
	return clockFunc(func() time.Time {
		return *t
	})
}

type clockFunc func() time.Time

func (f clockFunc) Now() time.Time {
	return f()
}
