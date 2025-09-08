// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package crossmodel

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-macaroon-bakery/macaroon-bakery/v3/bakery"
	"github.com/juju/tc"
	gomock "go.uber.org/mock/gomock"

	"github.com/juju/juju/apiserver/apiserverhttp"
)

type crossModelHandlerSuite struct {
	mux *apiserverhttp.Mux
	srv *httptest.Server

	authContext *MockOfferAuthContext
}

func TestObjectsHandlerSuite(t *testing.T) {
	tc.Run(t, &crossModelHandlerSuite{})
}

func (s *crossModelHandlerSuite) SetUpTest(c *tc.C) {
	s.mux = apiserverhttp.NewMux()
	s.srv = httptest.NewServer(s.mux)
}

func (s *crossModelHandlerSuite) TearDownTest(c *tc.C) {
	s.srv.Close()
}

func (s *crossModelHandlerSuite) TestAddOfferAuthHandlers(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.authContext.EXPECT().OfferThirdPartyKey().Return(bakery.MustGenerateKey())

	err := AddOfferAuthHandlers(s.authContext, s.mux)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *crossModelHandlerSuite) TestAddOfferAuthHandlersRegisterTwice(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.authContext.EXPECT().OfferThirdPartyKey().Return(bakery.MustGenerateKey()).Times(2)

	err := AddOfferAuthHandlers(s.authContext, s.mux)
	c.Assert(err, tc.ErrorIsNil)

	err = AddOfferAuthHandlers(s.authContext, s.mux)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *crossModelHandlerSuite) TestServePOST(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.authContext.EXPECT().OfferThirdPartyKey().Return(bakery.MustGenerateKey())

	err := AddOfferAuthHandlers(s.authContext, s.mux)
	c.Assert(err, tc.ErrorIsNil)

	resp, err := http.Post(s.srv.URL+localOfferAccessLocationPath+"/discharge", "application/octet-stream", nil)
	c.Assert(err, tc.ErrorIsNil)
	defer func() { _ = resp.Body.Close() }()
	c.Assert(resp.StatusCode, tc.Not(tc.Equals), http.StatusNotFound)
}

func (s *crossModelHandlerSuite) TestServeGET(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.authContext.EXPECT().OfferThirdPartyKey().Return(bakery.MustGenerateKey())

	err := AddOfferAuthHandlers(s.authContext, s.mux)
	c.Assert(err, tc.ErrorIsNil)

	resp, err := http.Get(s.srv.URL + localOfferAccessLocationPath + "/publickey")
	c.Assert(err, tc.ErrorIsNil)
	defer func() { _ = resp.Body.Close() }()
	c.Assert(resp.StatusCode, tc.Equals, http.StatusOK)
}

func (s *crossModelHandlerSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.authContext = NewMockOfferAuthContext(ctrl)

	c.Cleanup(func() {
		s.authContext = nil
	})

	return ctrl
}
