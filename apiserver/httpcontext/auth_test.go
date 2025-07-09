// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package httpcontext_test

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"github.com/juju/names/v6"
	"github.com/juju/tc"

	"github.com/juju/juju/apiserver/authentication"
	"github.com/juju/juju/apiserver/httpcontext"
	"github.com/juju/juju/internal/testhelpers"
	jujutesting "github.com/juju/juju/internal/testing"
)

type BasicAuthHandlerSuite struct {
	testhelpers.IsolationSuite
	stub     testhelpers.Stub
	handler  *httpcontext.AuthHandler
	authInfo authentication.AuthInfo
	server   *httptest.Server
}

func TestBasicAuthHandlerSuite(t *testing.T) {
	tc.Run(t, &BasicAuthHandlerSuite{})
}

func (s *BasicAuthHandlerSuite) SetUpTest(c *tc.C) {
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
		Entity:   &mockEntity{tag: names.NewUserTag("bob")},
		ModelTag: jujutesting.ModelTag,
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

func (s *BasicAuthHandlerSuite) TestRequestAuthInfoNoContext(c *tc.C) {
	_, ok := httpcontext.RequestAuthInfo(c.Context())
	c.Assert(ok, tc.IsFalse)
}

func (s *BasicAuthHandlerSuite) TestSuccess(c *tc.C) {
	resp, err := s.server.Client().Get(s.server.URL)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(resp.StatusCode, tc.Equals, http.StatusOK)
	defer resp.Body.Close()
	out, err := io.ReadAll(resp.Body)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(string(out), tc.Equals, "hullo!")
	s.stub.CheckCallNames(c, "Authenticate", "Authorize")
}

func (s *BasicAuthHandlerSuite) TestAuthenticationFailure(c *tc.C) {
	s.stub.SetErrors(errors.New("username/password invalid"))

	resp, err := s.server.Client().Get(s.server.URL)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(resp.StatusCode, tc.Equals, http.StatusUnauthorized)
	defer resp.Body.Close()

	out, err := io.ReadAll(resp.Body)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(string(out), tc.Equals, "authentication failed: username/password invalid\n")
	s.stub.CheckCallNames(c, "Authenticate")
}

func (s *BasicAuthHandlerSuite) TestAuthorizationFailure(c *tc.C) {
	s.stub.SetErrors(nil, errors.New("unauthorized access for resource"))

	resp, err := s.server.Client().Get(s.server.URL)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(resp.StatusCode, tc.Equals, http.StatusForbidden)
	defer resp.Body.Close()
	out, err := io.ReadAll(resp.Body)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(string(out), tc.Equals, "authorization failed: unauthorized access for resource\n")
	s.stub.CheckCallNames(c, "Authenticate", "Authorize")
}

func (s *BasicAuthHandlerSuite) TestAuthorizationOptional(c *tc.C) {
	s.handler.Authorizer = nil

	resp, err := s.server.Client().Get(s.server.URL)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(resp.StatusCode, tc.Equals, http.StatusOK)
	defer resp.Body.Close()
}

type mockEntity struct {
	tag names.Tag
}

func (e *mockEntity) Tag() names.Tag {
	return e.tag
}

type CompositeAuthSuite struct {
	testhelpers.IsolationSuite
}

func TestCompositeAuthSuite(t *testing.T) {
	tc.Run(t, &CompositeAuthSuite{})
}

type stubAuthorizer struct {
	expected authentication.AuthInfo
	err      error
}

func (a stubAuthorizer) Authorize(ctx context.Context, info authentication.AuthInfo) error {
	if !reflect.DeepEqual(a.expected, info) {
		return errors.New("wrong auth info")
	}
	return a.err
}

func (s *CompositeAuthSuite) TestAuthorizeSuccess(c *tc.C) {
	authInfo := authentication.AuthInfo{Controller: true}
	var auth httpcontext.CompositeAuthorizer = []authentication.Authorizer{
		stubAuthorizer{
			expected: authInfo,
			err:      errors.New("unauthorized"),
		},
		stubAuthorizer{
			expected: authInfo,
		},
	}
	err := auth.Authorize(context.Background(), authInfo)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *CompositeAuthSuite) TestAuthorizeFail(c *tc.C) {
	authInfo := authentication.AuthInfo{Controller: true}
	var auth httpcontext.CompositeAuthorizer = []authentication.Authorizer{
		stubAuthorizer{
			expected: authInfo,
			err:      errors.New("unauthorized"),
		},
		stubAuthorizer{
			expected: authInfo,
			err:      errors.New("unauthorized"),
		},
	}
	err := auth.Authorize(context.Background(), authInfo)
	c.Assert(err, tc.ErrorMatches, "permission denied")
}
