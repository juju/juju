// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package sshtunnel

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/juju/tc"
	"github.com/lestrrat-go/jwx/v3/jwt"

	"github.com/juju/juju/core/permission"
	"github.com/juju/juju/core/virtualhostname"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/sshproxy"
)

const testModelUUID = "8419cd78-4993-4c3a-928e-c646226beeee"

type relaySuite struct{}

func TestRelaySuite(t *testing.T) {
	tc.Run(t, &relaySuite{})
}

func (s *relaySuite) TestAdminAccessAuthorizes(c *tc.C) {
	resolver := &stubResolver{termination: sshproxy.Termination{}}
	w := s.serveRelay(c, resolver, testModelUUID, string(permission.AdminAccess))

	// Authorization passed: the resolver was called. The handler then
	// attempts to hijack the connection, which httptest.NewRecorder
	// does not support, so it logs and returns without writing a status.
	c.Check(w.Code, tc.Not(tc.Equals), http.StatusForbidden)
	c.Check(resolver.called, tc.IsTrue)
}

func (s *relaySuite) TestRelayAuthorization(c *tc.C) {
	for _, test := range []struct {
		name      string
		modelUUID string
		access    string
		wantCode  int
	}{
		{"read denied", testModelUUID, string(permission.ReadAccess), http.StatusForbidden},
		{"write denied", testModelUUID, string(permission.WriteAccess), http.StatusForbidden},
		{"wrong model denied", "99999999-9999-9999-9999-999999999999", string(permission.AdminAccess), http.StatusForbidden},
		{"invalid permission rejected", testModelUUID, string(permission.SuperuserAccess), http.StatusInternalServerError},
	} {
		c.Run(test.name, func(t *testing.T) {
			resolver := &stubResolver{}
			w := s.serveRelay(c, resolver, test.modelUUID, test.access)
			tc.Check(t, w.Code, tc.Equals, test.wantCode)
			tc.Check(t, resolver.called, tc.IsFalse)
		})
	}
}

func (s *relaySuite) TestMissingJWTUnauthorized(c *tc.C) {
	resolver := &stubResolver{}
	w := s.serveRelay(c, resolver, testModelUUID, "", withoutToken())
	c.Check(w.Code, tc.Equals, http.StatusUnauthorized)
	c.Check(resolver.called, tc.IsFalse)
}

func (s *relaySuite) newHandler(c *tc.C, resolver *stubResolver) *RelayHandler {
	handler, err := NewRelayHandler(RelayHandlerConfig{
		Logger:   loggertesting.WrapCheckLog(c),
		Resolver: resolver,
		Metrics:  &stubMetrics{},
	})
	c.Assert(err, tc.ErrorIsNil)
	return handler
}

// serveRelay dispatches a relay request and returns the response recorder.
// A non-empty access builds a token granting that permission on modelUUID;
// withoutToken() suppresses the JWT entirely.
func (s *relaySuite) serveRelay(c *tc.C, resolver *stubResolver, modelUUID, access string, opts ...serveOpt) *httptest.ResponseRecorder {
	var token jwt.Token
	if access != "" {
		token = newRelayToken(c, modelUUID, access)
	}
	for _, opt := range opts {
		opt(&token)
	}
	destination := newMachineDestination(c, testModelUUID)
	r := httptest.NewRequest(http.MethodGet, "/ssh-relay/"+destination.String(), nil)
	if token != nil {
		r = r.WithContext(context.WithValue(r.Context(), RelayJWTKey{}, token))
	}
	r.URL.RawQuery = ":virtualHostname=" + destination.String()

	w := httptest.NewRecorder()
	s.newHandler(c, resolver).ServeHTTP(w, r)
	return w
}

type serveOpt func(*jwt.Token)

func withoutToken() serveOpt {
	return func(t *jwt.Token) { *t = nil }
}

// newRelayToken builds a JWT with an access claim granting the given
// permission on the given model tag.
func newRelayToken(c *tc.C, modelUUID, access string) jwt.Token {
	token, err := jwt.NewBuilder().
		Audience([]string{"test-controller"}).
		Subject("user-admin@external").
		Issuer("test").
		JwtID(uuid.NewString()).
		Claim("access", map[string]any{
			"model-" + modelUUID: access,
		}).
		Build()
	c.Assert(err, tc.ErrorIsNil)
	return token
}

func newMachineDestination(c *tc.C, modelUUID string) virtualhostname.Info {
	info, err := virtualhostname.NewInfoMachineTarget(modelUUID, "0")
	c.Assert(err, tc.ErrorIsNil)
	return info
}

type stubResolver struct {
	termination sshproxy.Termination
	err         error
	called      bool
}

func (r *stubResolver) Resolve(_ context.Context, _ virtualhostname.Info) (sshproxy.Termination, error) {
	r.called = true
	return r.termination, r.err
}

type stubMetrics struct {
	count int
}

func (m *stubMetrics) IncConnectionCount(string) { m.count++ }
func (m *stubMetrics) DecConnectionCount(string) { m.count-- }
