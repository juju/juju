// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver_test

import (
	"io"
	"net/http"
	"strings"

	"github.com/go-macaroon-bakery/macaroon-bakery/v3/bakery"
	"github.com/juju/testing"
	gomock "go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver"
	"github.com/juju/juju/apiserver/mocks"
)

type bakerySuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&bakerySuite{})

func (s *bakerySuite) TestGetLocalOfferBakery(c *gc.C) {
	ctrl := gomock.NewController(c)

	mockRoundTripper := mocks.NewMockRoundTripper(ctrl)
	s.PatchValue(&apiserver.DefaultTransport, mockRoundTripper)
	mockBakeryConfig := mocks.NewMockBakeryConfig(ctrl)
	mockExpirableStorage := mocks.NewMockExpirableStorage(ctrl)
	mockFirstPartyCaveatChecker := mocks.NewMockFirstPartyCaveatChecker(ctrl)

	key, err := bakery.GenerateKey()
	c.Assert(err, gc.IsNil)
	mockBakeryConfig.EXPECT().GetOffersThirdPartyKey().Return(key, nil)
	mockFirstPartyCaveatChecker.EXPECT().Namespace().Return(nil)

	b, err := apiserver.GetLocalOfferBakery("", mockBakeryConfig, mockExpirableStorage, mockFirstPartyCaveatChecker)
	c.Assert(err, gc.IsNil)
	c.Assert(b, gc.NotNil)
}

func (s *bakerySuite) TestGetJaaSOfferBakery(c *gc.C) {
	ctrl := gomock.NewController(c)

	mockRoundTripper := mocks.NewMockRoundTripper(ctrl)
	s.PatchValue(&apiserver.DefaultTransport, mockRoundTripper)
	mockBakeryConfig := mocks.NewMockBakeryConfig(ctrl)
	mockExpirableStorage := mocks.NewMockExpirableStorage(ctrl)
	mockFirstPartyCaveatChecker := mocks.NewMockFirstPartyCaveatChecker(ctrl)

	key, err := bakery.GenerateKey()
	c.Assert(err, gc.IsNil)
	mockBakeryConfig.EXPECT().GetExternalUsersThirdPartyKey().Return(key, nil)
	mockExpirableStorage.EXPECT().ExpireAfter(gomock.Any()).Return(mockExpirableStorage)
	mockFirstPartyCaveatChecker.EXPECT().Namespace().Return(nil)
	mockRoundTripper.EXPECT().RoundTrip(gomock.Any()).DoAndReturn(
		func(req *http.Request) (*http.Response, error) {
			req.Header.Set("Content-Type", "application/json")
			c.Assert(req.URL.String(), gc.Equals, `https://example.com/macaroons/discharge/info`)
			resp := &http.Response{
				Request:    req,
				StatusCode: http.StatusOK,
				Body: io.NopCloser(
					strings.NewReader(
						`{"PublicKey": "AhIuwQfV71m2G+DhE/YNT1jIbSvp6jWgivTf06+tLBU=", "Version": 3}`,
					),
				),
			}
			resp.Header = req.Header
			return resp, nil
		},
	)

	b, url, err := apiserver.GetJaaSOfferBakery(
		"https://example.com/.well-known/jwks.json", "",
		mockBakeryConfig, mockExpirableStorage, mockFirstPartyCaveatChecker,
	)
	c.Assert(err, gc.IsNil)
	c.Assert(b, gc.NotNil)
	c.Assert(url, gc.Equals, "https://example.com/macaroons")
}
