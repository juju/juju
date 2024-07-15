// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"

	"github.com/go-macaroon-bakery/macaroon-bakery/v3/bakery"
	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"
)

type configServiceSuite struct {
	st *MockState
}

var _ = gc.Suite(&configServiceSuite{})

func (s *configServiceSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.st = NewMockState(ctrl)
	return ctrl
}

func (s *configServiceSuite) TestInitialise(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.st.EXPECT().InitialiseBakeryConfig(
		gomock.Any(),
		gomock.AssignableToTypeOf(&bakery.KeyPair{}),
		gomock.AssignableToTypeOf(&bakery.KeyPair{}),
		gomock.AssignableToTypeOf(&bakery.KeyPair{}),
		gomock.AssignableToTypeOf(&bakery.KeyPair{}),
	).Return(nil)

	srv := NewBakeryConfigService(s.st)
	err := srv.InitialiseBakeryConfig(context.Background())
	c.Assert(err, jc.ErrorIsNil)
}

func (s *configServiceSuite) TestGetLocalUsersKey(c *gc.C) {
	defer s.setupMocks(c).Finish()

	testKey := bakery.MustGenerateKey()
	s.st.EXPECT().GetLocalUsersKey(gomock.Any()).Return(testKey, nil)

	srv := NewBakeryConfigService(s.st)
	key, err := srv.GetLocalUsersKey(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(key, gc.DeepEquals, testKey)
}

func (s *configServiceSuite) TestGetLocalUsersThirdPartyKey(c *gc.C) {
	defer s.setupMocks(c).Finish()

	testKey := bakery.MustGenerateKey()
	s.st.EXPECT().GetLocalUsersThirdPartyKey(gomock.Any()).Return(testKey, nil)

	srv := NewBakeryConfigService(s.st)
	key, err := srv.GetLocalUsersThirdPartyKey(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(key, gc.DeepEquals, testKey)
}

func (s *configServiceSuite) TestGetExternalUsersThirdPartyKey(c *gc.C) {
	defer s.setupMocks(c).Finish()

	testKey := bakery.MustGenerateKey()
	s.st.EXPECT().GetExternalUsersThirdPartyKey(gomock.Any()).Return(testKey, nil)

	srv := NewBakeryConfigService(s.st)
	key, err := srv.GetExternalUsersThirdPartyKey(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(key, gc.DeepEquals, testKey)
}

func (s *configServiceSuite) TestGetOffersThirdPartyKey(c *gc.C) {
	defer s.setupMocks(c).Finish()

	testKey := bakery.MustGenerateKey()
	s.st.EXPECT().GetOffersThirdPartyKey(gomock.Any()).Return(testKey, nil)

	srv := NewBakeryConfigService(s.st)
	key, err := srv.GetOffersThirdPartyKey(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(key, gc.DeepEquals, testKey)
}
