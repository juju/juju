// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"
	"time"

	"github.com/go-macaroon-bakery/macaroon-bakery/v3/bakery"
	"github.com/go-macaroon-bakery/macaroon-bakery/v3/bakery/dbrootkeystore"
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/domain/macaroon"
	macaroonerrors "github.com/juju/juju/domain/macaroon/errors"
)

var _ dbrootkeystore.Backing = &RootKeyService{}
var _ dbrootkeystore.ContextBacking = &RootKeyService{}

type rootKeyServiceSuite struct {
	st *MockState
}

var _ = gc.Suite(&rootKeyServiceSuite{})

var key = dbrootkeystore.RootKey{
	Id:      []byte("0"),
	Created: time.Now(),
	Expires: time.Now().Add(2 * time.Second),
	RootKey: []byte("key0"),
}

func (s *rootKeyServiceSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.st = NewMockState(ctrl)
	return ctrl
}

func (s *rootKeyServiceSuite) TestStubMethods(c *gc.C) {
	defer s.setupMocks(c).Finish()

	srv := NewRootKeyService(s.st)

	_, err := srv.GetKey([]byte("0"))
	c.Assert(err, jc.ErrorIs, errors.NotImplemented)

	_, err = srv.FindLatestKey(time.Now(), time.Now(), time.Now())
	c.Assert(err, jc.ErrorIs, errors.NotImplemented)

	err = srv.InsertKey(dbrootkeystore.RootKey{})
	c.Assert(err, jc.ErrorIs, errors.NotImplemented)
}

func (s *rootKeyServiceSuite) TestGetKeyContext(c *gc.C) {
	defer s.setupMocks(c).Finish()

	id := []byte("0")
	s.st.EXPECT().GetKey(gomock.Any(), id).Return(encodeRootKey(key), nil)
	srv := NewRootKeyService(s.st)

	res, err := srv.GetKeyContext(context.Background(), id)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(res, gc.DeepEquals, key)
}

func (s *rootKeyServiceSuite) TestGetKeyContextNotFound(c *gc.C) {
	defer s.setupMocks(c).Finish()

	id := []byte("0")
	s.st.EXPECT().GetKey(gomock.Any(), id).Return(macaroon.RootKey{}, macaroonerrors.KeyNotFound)
	srv := NewRootKeyService(s.st)

	_, err := srv.GetKeyContext(context.Background(), id)
	c.Assert(err, jc.ErrorIs, bakery.ErrNotFound)
}

func (s *rootKeyServiceSuite) TestFindLatestKeyContext(c *gc.C) {
	defer s.setupMocks(c).Finish()

	createdAfter := time.Now()
	expiresAfter := time.Now().Add(-time.Second)
	expiresBefore := time.Now().Add(time.Second)
	s.st.EXPECT().FindLatestKey(gomock.Any(), createdAfter, expiresAfter, expiresBefore).Return(encodeRootKey(key), nil)
	srv := NewRootKeyService(s.st)

	res, err := srv.FindLatestKeyContext(context.Background(), createdAfter, expiresAfter, expiresBefore)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(res, gc.DeepEquals, key)
}

func (s *rootKeyServiceSuite) TestFindLatestKeyContextNotFound(c *gc.C) {
	defer s.setupMocks(c).Finish()

	createdAfter := time.Now()
	expiresAfter := time.Now().Add(-time.Second)
	expiresBefore := time.Now().Add(time.Second)
	s.st.EXPECT().FindLatestKey(gomock.Any(), createdAfter, expiresAfter, expiresBefore).Return(macaroon.RootKey{}, macaroonerrors.KeyNotFound)
	srv := NewRootKeyService(s.st)

	res, err := srv.FindLatestKeyContext(context.Background(), createdAfter, expiresAfter, expiresBefore)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(res, gc.DeepEquals, dbrootkeystore.RootKey{})
}

func (s *rootKeyServiceSuite) TestInsertKeyContext(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.st.EXPECT().InsertKey(gomock.Any(), encodeRootKey(key))
	srv := NewRootKeyService(s.st)

	err := srv.InsertKeyContext(context.Background(), key)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *rootKeyServiceSuite) TestInsertKeyContextError(c *gc.C) {
	defer s.setupMocks(c).Finish()

	boom := errors.Errorf("boom")
	s.st.EXPECT().InsertKey(gomock.Any(), encodeRootKey(key)).Return(boom)
	srv := NewRootKeyService(s.st)

	err := srv.InsertKeyContext(context.Background(), key)
	c.Assert(err, gc.Equals, boom)
}
