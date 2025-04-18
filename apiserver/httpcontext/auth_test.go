// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package httpcontext_test

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"

	"github.com/juju/names/v6"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/authentication"
	"github.com/juju/juju/apiserver/httpcontext"
)

type BasicAuthHandlerSuite struct {
	testing.IsolationSuite
	stub     testing.Stub
	handler  *httpcontext.AuthHandler
	authInfo authentication.AuthInfo
	server   *httptest.Server
}

var _ = gc.Suite(&BasicAuthHandlerSuite{})

func (s *BasicAuthHandlerSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)
	s.stub.ResetCalls()
	s.handler = &httpcontext.AuthHandler{
		Authenticator: s,
		Authorizer:    s,
		NextHandler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			authInfo, ok := httpcontext.RequestAuthInfo(r.Context())
			if !ok || authInfo.Entity != s.authInfo.Entity {
				w.WriteHeader(http.StatusBadRequest)
			} else {
				w.WriteHeader(http.StatusOK)
				io.WriteString(w, "hullo!")
			}
		}),
	}
	s.server = httptest.NewServer(s.handler)
	s.authInfo = authentication.AuthInfo{
		Entity: &mockEntity{tag: names.NewUserTag("bob")},
	}
}

func (s *BasicAuthHandlerSuite) Authenticate(req *http.Request) (authentication.AuthInfo, error) {
	s.stub.MethodCall(s, "Authenticate", req)
	if err := s.stub.NextErr(); err != nil {
		return authentication.AuthInfo{}, err
	}
	return s.authInfo, nil
}

func (s *BasicAuthHandlerSuite) AuthenticateLoginRequest(
	_ context.Context,
	_,
	_ string,
	_ authentication.AuthParams,
) (authentication.AuthInfo, error) {
	panic("should not be called")
}

func (s *BasicAuthHandlerSuite) Authorize(ctx context.Context, authInfo authentication.AuthInfo) error {
	s.stub.MethodCall(s, "Authorize", authInfo)
	return s.stub.NextErr()
}

func (s *BasicAuthHandlerSuite) TestRequestAuthInfoNoContext(c *gc.C) {
	_, ok := httpcontext.RequestAuthInfo(context.Background())
	c.Assert(ok, jc.IsFalse)
}

func (s *BasicAuthHandlerSuite) TestSuccess(c *gc.C) {
	resp, err := s.server.Client().Get(s.server.URL)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(resp.StatusCode, gc.Equals, http.StatusOK)
	defer resp.Body.Close()
	out, err := io.ReadAll(resp.Body)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(string(out), gc.Equals, "hullo!")
	s.stub.CheckCallNames(c, "Authenticate", "Authorize")
}

func (s *BasicAuthHandlerSuite) TestAuthenticationFailure(c *gc.C) {
	s.stub.SetErrors(errors.New("username/password invalid"))

	resp, err := s.server.Client().Get(s.server.URL)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(resp.StatusCode, gc.Equals, http.StatusUnauthorized)
	defer resp.Body.Close()

	out, err := io.ReadAll(resp.Body)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(string(out), gc.Equals, "authentication failed: username/password invalid\n")
	s.stub.CheckCallNames(c, "Authenticate")
}

func (s *BasicAuthHandlerSuite) TestAuthorizationFailure(c *gc.C) {
	s.stub.SetErrors(nil, errors.New("unauthorized access for resource"))

	resp, err := s.server.Client().Get(s.server.URL)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(resp.StatusCode, gc.Equals, http.StatusForbidden)
	defer resp.Body.Close()
	out, err := io.ReadAll(resp.Body)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(string(out), gc.Equals, "authorization failed: unauthorized access for resource\n")
	s.stub.CheckCallNames(c, "Authenticate", "Authorize")
}

func (s *BasicAuthHandlerSuite) TestAuthorizationOptional(c *gc.C) {
	s.handler.Authorizer = nil

	resp, err := s.server.Client().Get(s.server.URL)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(resp.StatusCode, gc.Equals, http.StatusOK)
	defer resp.Body.Close()
}

type mockEntity struct {
	tag names.Tag
}

func (e *mockEntity) Tag() names.Tag {
	return e.tag
}
