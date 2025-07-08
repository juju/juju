// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package httpcontext_test

import (
	"context"
	"errors"
	"io"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"reflect"

	"github.com/juju/names/v4"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/httpcontext"
	"github.com/juju/juju/rpc/params"
	jujutesting "github.com/juju/juju/testing"
)

type BasicAuthHandlerSuite struct {
	testing.IsolationSuite
	stub     testing.Stub
	handler  *httpcontext.BasicAuthHandler
	authInfo httpcontext.AuthInfo
	server   *httptest.Server
}

var _ = gc.Suite(&BasicAuthHandlerSuite{})

func (s *BasicAuthHandlerSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)
	s.stub.ResetCalls()
	s.handler = &httpcontext.BasicAuthHandler{
		Authenticator: s,
		Authorizer:    s,
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			authInfo, ok := httpcontext.RequestAuthInfo(r)
			if !ok || authInfo != s.authInfo {
				w.WriteHeader(http.StatusBadRequest)
			} else {
				w.WriteHeader(http.StatusOK)
				io.WriteString(w, "hullo!")
			}
		}),
	}
	s.server = httptest.NewServer(s.handler)
	s.authInfo = httpcontext.AuthInfo{
		Entity:   &mockEntity{tag: names.NewUserTag("bob")},
		ModelTag: jujutesting.ModelTag,
	}
}

func (s *BasicAuthHandlerSuite) Authenticate(req *http.Request) (httpcontext.AuthInfo, error) {
	s.stub.MethodCall(s, "Authenticate", req)
	if err := s.stub.NextErr(); err != nil {
		return httpcontext.AuthInfo{}, err
	}
	return s.authInfo, nil
}

func (s *BasicAuthHandlerSuite) AuthenticateLoginRequest(
	ctx context.Context, serverHost, modelUUID string, req params.LoginRequest,
) (httpcontext.AuthInfo, error) {
	panic("should not be called")
}

func (s *BasicAuthHandlerSuite) Authorize(authInfo httpcontext.AuthInfo) error {
	s.stub.MethodCall(s, "Authorize", authInfo)
	return s.stub.NextErr()
}

func (s *BasicAuthHandlerSuite) TestRequestAuthInfoNoContext(c *gc.C) {
	_, ok := httpcontext.RequestAuthInfo(&http.Request{})
	c.Assert(ok, jc.IsFalse)
}

func (s *BasicAuthHandlerSuite) TestSuccess(c *gc.C) {
	resp, err := s.server.Client().Get(s.server.URL)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(resp.StatusCode, gc.Equals, http.StatusOK)
	defer resp.Body.Close()
	out, err := ioutil.ReadAll(resp.Body)
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

	out, err := ioutil.ReadAll(resp.Body)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(string(out), gc.Equals, "authentication failed: username/password invalid\n")
	c.Assert(resp.Header.Get("WWW-Authenticate"), gc.Equals, `Basic realm="juju"`)
	s.stub.CheckCallNames(c, "Authenticate")
}

func (s *BasicAuthHandlerSuite) TestAuthorizationFailure(c *gc.C) {
	s.stub.SetErrors(nil, errors.New("unauthorized access for resource"))

	resp, err := s.server.Client().Get(s.server.URL)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(resp.StatusCode, gc.Equals, http.StatusForbidden)
	defer resp.Body.Close()
	out, err := ioutil.ReadAll(resp.Body)
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

type CompositeAuthSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&CompositeAuthSuite{})

type stubAuthorizer struct {
	expected httpcontext.AuthInfo
	err      error
}

func (a stubAuthorizer) Authorize(info httpcontext.AuthInfo) error {
	if !reflect.DeepEqual(a.expected, info) {
		return errors.New("wrong auth info")
	}
	return a.err
}

func (s *CompositeAuthSuite) TestAuthorizeSuccess(c *gc.C) {
	authInfo := httpcontext.AuthInfo{Controller: true}
	var auth httpcontext.CompositeAuthorizer = []httpcontext.Authorizer{
		stubAuthorizer{
			expected: authInfo,
			err:      errors.New("unauthorized"),
		},
		stubAuthorizer{
			expected: authInfo,
		},
	}
	err := auth.Authorize(authInfo)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *CompositeAuthSuite) TestAuthorizeFail(c *gc.C) {
	authInfo := httpcontext.AuthInfo{Controller: true}
	var auth httpcontext.CompositeAuthorizer = []httpcontext.Authorizer{
		stubAuthorizer{
			expected: authInfo,
			err:      errors.New("unauthorized"),
		},
		stubAuthorizer{
			expected: authInfo,
			err:      errors.New("unauthorized"),
		},
	}
	err := auth.Authorize(authInfo)
	c.Assert(err, gc.ErrorMatches, "permission denied")
}
