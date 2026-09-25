// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/juju/names/v6"
	"github.com/juju/tc"
	"github.com/lestrrat-go/jwx/v3/jwt"

	"github.com/juju/juju/apiserver/authentication"
	authjwt "github.com/juju/juju/apiserver/authentication/jwt"
	"github.com/juju/juju/apiserver/httpcontext"
	sshproxy "github.com/juju/juju/apiserver/internal/handlers/sshproxy"
	"github.com/juju/juju/core/permission"
)

type relayAuthSuite struct{}

func TestRelayAuthSuite(t *testing.T) {
	tc.Run(t, &relayAuthSuite{})
}

// newRelayAuthInfo returns auth info carrying the given delegator, as the
// HTTP authentication layer would produce for a bearer-JWT request.
func newRelayAuthInfo(delegator authentication.PermissionDelegator) authentication.AuthInfo {
	return authentication.AuthInfo{
		Tag:                       names.NewUserTag("alice"),
		Delegator:                 delegator,
		IsExternallyAuthenticated: delegator != nil,
	}
}

// withAuthInfo injects auth info into the request context by running the
// request through a real httpcontext.AuthHandler with a stub authenticator,
// so the production authInfoKey is used.
func withAuthInfo(t *testing.T, r *http.Request, authInfo authentication.AuthInfo) *http.Request {
	var out *http.Request
	authHandler := &httpcontext.AuthHandler{
		NextHandler: http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
			out = r
		}),
		Authenticator: stubHTTPAuthenticator{authInfo: authInfo},
		Authorizer:    authentication.AuthorizerFunc(func(context.Context, authentication.AuthInfo) error { return nil }),
	}
	w := httptest.NewRecorder()
	authHandler.ServeHTTP(w, r)
	tc.Assert(t, w.Code, tc.Equals, http.StatusOK)
	return out
}

// stubHTTPAuthenticator returns a fixed auth info.
type stubHTTPAuthenticator struct {
	authInfo authentication.AuthInfo
}

// Authenticate implements authentication.HTTPAuthenticator.
func (s stubHTTPAuthenticator) Authenticate(*http.Request) (authentication.AuthInfo, error) {
	return s.authInfo, nil
}

// captureJWT is a handler that records the JWT injected into the request
// context by the wrapper, if any.
type captureJWT struct {
	token jwt.Token
	ok    bool
}

// ServeHTTP implements http.Handler.
func (h *captureJWT) ServeHTTP(_ http.ResponseWriter, r *http.Request) {
	h.token, h.ok = r.Context().Value(sshproxy.RelayJWTKey{}).(jwt.Token)
}

func (s *relayAuthSuite) newToken(c *tc.C) jwt.Token {
	token, err := jwt.NewBuilder().Subject("user-alice").Build()
	c.Assert(err, tc.ErrorIsNil)
	return token
}

func (s *relayAuthSuite) TestRelayWrapperInjectsDelegatorToken(c *tc.C) {
	srv := &Server{}
	token := s.newToken(c)

	r := httptest.NewRequest(http.MethodGet, "/ssh-relay/x.juju.local", nil)
	r = withAuthInfo(c.T, r, newRelayAuthInfo(&authjwt.PermissionDelegator{Token: token}))

	captured := &captureJWT{}
	wrapper := srv.sshRelayRequestWrapper(captured)
	wrapper.ServeHTTP(httptest.NewRecorder(), r)

	c.Check(captured.ok, tc.IsTrue)
	c.Check(captured.token, tc.Equals, token)
}

func (s *relayAuthSuite) TestRelayWrapperRejectsUnauthorizedAuthInfo(c *tc.C) {
	srv := &Server{}
	captured := &captureJWT{}
	wrapper := srv.sshRelayRequestWrapper(captured)

	for _, test := range []struct {
		name     string
		authInfo authentication.AuthInfo
	}{
		{"missing auth info", authentication.AuthInfo{}},
		{"non-JWT delegator", newRelayAuthInfo(stubPermissionDelegator{})},
		{"nil delegator", newRelayAuthInfo(nil)},
	} {
		c.Run(test.name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodGet, "/ssh-relay/x.juju.local", nil)
			if test.authInfo.Delegator != nil || test.authInfo.Tag != nil {
				r = withAuthInfo(t, r, test.authInfo)
			}

			w := httptest.NewRecorder()
			wrapper.ServeHTTP(w, r)

			tc.Check(t, w.Code, tc.Equals, http.StatusUnauthorized)
			tc.Check(t, captured.ok, tc.IsFalse)
		})
	}
}

// stubPermissionDelegator is a PermissionDelegator that is not a
// *jwt.PermissionDelegator.
type stubPermissionDelegator struct{}

// SubjectPermissions implements authentication.PermissionDelegator.
func (stubPermissionDelegator) SubjectPermissions(context.Context, string, permission.ID) (permission.Access, error) {
	return permission.NoAccess, nil
}

// PermissionError implements authentication.PermissionDelegator.
func (stubPermissionDelegator) PermissionError(names.Tag, permission.Access) error {
	return nil
}

func (s *relayAuthSuite) TestRelayJWTAuthorizerAcceptsExternalAuth(c *tc.C) {
	token := s.newToken(c)
	authInfo := newRelayAuthInfo(&authjwt.PermissionDelegator{Token: token})

	err := relayJWTAuthorizer{}.Authorize(context.Background(), authInfo)
	c.Check(err, tc.ErrorIsNil)
}

func (s *relayAuthSuite) TestRelayJWTAuthorizerRejectsNonExternalAuth(c *tc.C) {
	authInfo := newRelayAuthInfo(&authjwt.PermissionDelegator{Token: s.newToken(c)})
	authInfo.IsExternallyAuthenticated = false

	err := relayJWTAuthorizer{}.Authorize(context.Background(), authInfo)
	c.Check(err, tc.ErrorMatches, "authorization is not for an externally authenticated user")
}

func (s *relayAuthSuite) TestRelayJWTAuthorizerRejectsMissingDelegator(c *tc.C) {
	authInfo := newRelayAuthInfo(nil)
	authInfo.IsExternallyAuthenticated = true

	err := relayJWTAuthorizer{}.Authorize(context.Background(), authInfo)
	c.Check(err, tc.ErrorMatches, "authorization is missing a permission delegator")
}

// Compile-time check that the test exercises the production auth info
// retrieval path.
var _ = httpcontext.RequestAuthInfo
