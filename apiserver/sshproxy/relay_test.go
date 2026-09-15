// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package sshproxy

import (
	"context"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/canonical/gomock/gomock"
	"github.com/juju/errors"
	"github.com/juju/tc"
	"github.com/lestrrat-go/jwx/v3/jwt"

	"github.com/juju/juju/core/permission"
	"github.com/juju/juju/core/virtualhostname"
	loggertesting "github.com/juju/juju/internal/logger/testing"
)

const testModelUUID = "8419cd78-4993-4c3a-928e-c646226beeee"

type relaySuite struct{}

func TestRelaySuite(t *testing.T) {
	tc.Run(t, &relaySuite{})
}

func (s *relaySuite) TestAdminAccessAuthorizes(c *tc.C) {
	ctrl := gomock.NewController(c)
	factory := NewMockTerminatingServerFactory(ctrl)
	w := s.serveRelay(c, factory, testModelUUID, string(permission.AdminAccess))

	// Authorization passed: the handler attempts to hijack the connection,
	// which httptest.NewRecorder does not support, so it logs and returns
	// without writing a status. Resolution happens after the upgrade, so
	// the factory is not reached here.
	c.Check(w.Code, tc.Not(tc.Equals), http.StatusForbidden)
	ctrl.Finish()
}

func (s *relaySuite) TestResolveErrorWrittenToConn(c *tc.C) {
	ctrl := gomock.NewController(c)
	factory := NewMockTerminatingServerFactory(ctrl)
	factory.EXPECT().New(gomock.Any(), gomock.Any()).
		Return(nil, errors.New("no such destination"))
	metrics := NewMockMetricsCollector(ctrl)
	metrics.EXPECT().IncConnectionCount("relay")
	metrics.EXPECT().DecConnectionCount("relay")
	handler, err := NewRelayHandler(RelayHandlerConfig{
		Logger:        loggertesting.WrapCheckLog(c),
		ServerFactory: factory,
		Metrics:       metrics,
	})
	c.Assert(err, tc.ErrorIsNil)

	// Inject the verified JWT into the request context server-side, as the
	// apiserver's HTTP authentication layer would.
	token := newRelayToken(c, testModelUUID, string(permission.AdminAccess))
	injectJWT := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := context.WithValue(r.Context(), RelayJWTKey{}, token)
		handler.ServeHTTP(w, r.WithContext(ctx))
	})
	server := httptest.NewServer(injectJWT)
	c.Cleanup(server.Close)

	destination := newMachineDestination(c, testModelUUID)
	req, err := http.NewRequest(http.MethodGet, server.URL+"/ssh-relay/"+destination.String(), nil)
	c.Assert(err, tc.ErrorIsNil)
	req.Header.Set("Connection", "Upgrade")
	req.Header.Set("Upgrade", RelayUpgradeToken)
	req.URL.RawQuery = ":virtualHostname=" + destination.String()

	// Write the upgrade request over a raw connection. After the 101
	// response the hijacked connection stays open on the server side,
	// which writes the resolve error as pre-banner text before closing.
	conn, err := net.Dial("tcp", server.Listener.Addr().String())
	c.Assert(err, tc.ErrorIsNil)
	defer func() { _ = conn.Close() }()
	err = req.Write(conn)
	c.Assert(err, tc.ErrorIsNil)

	_ = conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	out, err := io.ReadAll(conn)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(string(out), tc.Contains, "juju ssh relay: no such destination")
	ctrl.Finish()
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
			ctrl := gomock.NewController(t)
			factory := NewMockTerminatingServerFactory(ctrl)
			w := s.serveRelay(c, factory, test.modelUUID, test.access)
			tc.Check(t, w.Code, tc.Equals, test.wantCode)
			ctrl.Finish()
		})
	}
}

func (s *relaySuite) TestMissingJWTUnauthorized(c *tc.C) {
	ctrl := gomock.NewController(c)
	factory := NewMockTerminatingServerFactory(ctrl)
	w := s.serveRelay(c, factory, testModelUUID, "")
	c.Check(w.Code, tc.Equals, http.StatusUnauthorized)
	ctrl.Finish()
}

func (s *relaySuite) newHandler(c *tc.C, factory TerminatingServerFactory) *RelayHandler {
	handler, err := NewRelayHandler(RelayHandlerConfig{
		Logger:        loggertesting.WrapCheckLog(c),
		ServerFactory: factory,
		Metrics:       NewMockMetricsCollector(gomock.NewController(c)),
	})
	c.Assert(err, tc.ErrorIsNil)
	return handler
}

// serveRelay dispatches a relay request and returns the response recorder.
// An empty access produces no JWT, testing the missing-token path.
func (s *relaySuite) serveRelay(c *tc.C, factory TerminatingServerFactory, modelUUID, access string) *httptest.ResponseRecorder {
	var token jwt.Token
	if access != "" {
		token = newRelayToken(c, modelUUID, access)
	}
	destination := newMachineDestination(c, testModelUUID)
	r := httptest.NewRequest(http.MethodGet, "/ssh-relay/"+destination.String(), nil)
	if token != nil {
		r = r.WithContext(context.WithValue(r.Context(), RelayJWTKey{}, token))
	}
	r.URL.RawQuery = ":virtualHostname=" + destination.String()

	w := httptest.NewRecorder()
	s.newHandler(c, factory).ServeHTTP(w, r)
	return w
}

func newRelayToken(c *tc.C, modelUUID, access string) jwt.Token {
	token, err := jwt.NewBuilder().
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
