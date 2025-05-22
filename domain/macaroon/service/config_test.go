// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"testing"

	"github.com/go-macaroon-bakery/macaroon-bakery/v3/bakery"
	"github.com/juju/tc"
	"go.uber.org/mock/gomock"
)

type configServiceSuite struct {
	st *MockState
}

func TestConfigServiceSuite(t *testing.T) {
	tc.Run(t, &configServiceSuite{})
}

func (s *configServiceSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.st = NewMockState(ctrl)
	return ctrl
}

func (s *configServiceSuite) TestInitialise(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.st.EXPECT().InitialiseBakeryConfig(
		gomock.Any(),
		gomock.AssignableToTypeOf(&bakery.KeyPair{}),
		gomock.AssignableToTypeOf(&bakery.KeyPair{}),
		gomock.AssignableToTypeOf(&bakery.KeyPair{}),
		gomock.AssignableToTypeOf(&bakery.KeyPair{}),
	).Return(nil)

	srv := NewBakeryConfigService(s.st)
	err := srv.InitialiseBakeryConfig(c.Context())
	c.Assert(err, tc.ErrorIsNil)
}

func (s *configServiceSuite) TestGetLocalUsersKey(c *tc.C) {
	defer s.setupMocks(c).Finish()

	testKey := bakery.MustGenerateKey()
	s.st.EXPECT().GetLocalUsersKey(gomock.Any()).Return(testKey, nil)

	srv := NewBakeryConfigService(s.st)
	key, err := srv.GetLocalUsersKey(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(key, tc.DeepEquals, testKey)
}

func (s *configServiceSuite) TestGetLocalUsersThirdPartyKey(c *tc.C) {
	defer s.setupMocks(c).Finish()

	testKey := bakery.MustGenerateKey()
	s.st.EXPECT().GetLocalUsersThirdPartyKey(gomock.Any()).Return(testKey, nil)

	srv := NewBakeryConfigService(s.st)
	key, err := srv.GetLocalUsersThirdPartyKey(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(key, tc.DeepEquals, testKey)
}

func (s *configServiceSuite) TestGetExternalUsersThirdPartyKey(c *tc.C) {
	defer s.setupMocks(c).Finish()

	testKey := bakery.MustGenerateKey()
	s.st.EXPECT().GetExternalUsersThirdPartyKey(gomock.Any()).Return(testKey, nil)

	srv := NewBakeryConfigService(s.st)
	key, err := srv.GetExternalUsersThirdPartyKey(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(key, tc.DeepEquals, testKey)
}

func (s *configServiceSuite) TestGetOffersThirdPartyKey(c *tc.C) {
	defer s.setupMocks(c).Finish()

	testKey := bakery.MustGenerateKey()
	s.st.EXPECT().GetOffersThirdPartyKey(gomock.Any()).Return(testKey, nil)

	srv := NewBakeryConfigService(s.st)
	key, err := srv.GetOffersThirdPartyKey(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(key, tc.DeepEquals, testKey)
}
