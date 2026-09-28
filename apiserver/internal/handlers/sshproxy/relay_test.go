// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package sshproxy

import (
	"context"
	"fmt"
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
	gliderssh "github.com/tailscale/gliderssh"
	gossh "golang.org/x/crypto/ssh"

	"github.com/juju/juju/core/permission"
	coresshproxy "github.com/juju/juju/core/sshproxy"
	"github.com/juju/juju/core/virtualhostname"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/pki/test"
)

const testModelUUID = "8419cd78-4993-4c3a-928e-c646226beeee"

type relaySuite struct{}

func TestRelaySuite(t *testing.T) {
	tc.Run(t, &relaySuite{})
}

// TestRelaySessionServedEndToEnd exercises the full relay flow over a real
// connection: upgrade, destination resolution, and an SSH session served
// by the terminating server built by the factory.
func (s *relaySuite) TestRelaySessionServedEndToEnd(c *tc.C) {
	ctrl := gomock.NewController(c)
	factory := NewMockTerminatingServerFactory(ctrl)

	// Build a real terminating server with a session handler that greets
	// the user, and a host key so the SSH handshake can complete.
	sessionDone := make(chan struct{})
	handlers := &stubProxyHandlers{sessionDone: sessionDone}
	terminatingServer := gliderssh.Server{
		ChannelHandlers: map[string]gliderssh.ChannelHandler{
			"session": gliderssh.DefaultSessionHandler,
		},
		Handler: handlers.SessionHandler,
	}
	privateKey, err := test.InsecureKeyProfile()
	c.Assert(err, tc.ErrorIsNil)
	signer, err := gossh.NewSignerFromSigner(privateKey)
	c.Assert(err, tc.ErrorIsNil)
	terminatingServer.AddHostKey(signer)

	destination := newMachineDestination(c, testModelUUID)
	factory.EXPECT().New(gomock.Any(), destination).Return(&terminatingServer, nil)

	conn, req := s.dialRelay(c, factory)

	upgraded, err := coresshproxy.PerformUpgrade(req, conn)
	c.Assert(err, tc.ErrorIsNil)

	// Run an SSH session over the upgraded connection. The terminating
	// server has no auth handlers, so gliderssh serves the connection
	// with no client auth, as it does for relayed sessions.
	sshConfig := &gossh.ClientConfig{
		HostKeyCallback: gossh.InsecureIgnoreHostKey(),
		Timeout:         5 * time.Second,
	}
	clientConn, chans, reqs, err := gossh.NewClientConn(upgraded, "relay", sshConfig)
	c.Assert(err, tc.ErrorIsNil)
	client := gossh.NewClient(clientConn, chans, reqs)
	defer func() { _ = client.Close() }()

	session, err := client.NewSession()
	c.Assert(err, tc.ErrorIsNil)
	defer func() { _ = session.Close() }()

	output, err := session.Output("")
	c.Assert(err, tc.ErrorIsNil)
	c.Check(string(output), tc.Equals, "relayed session\n")

	// The session handler has run, so the relay served the connection
	// end to end.
	select {
	case <-sessionDone:
	case <-time.After(5 * time.Second):
		c.Fatal("timed out waiting for the relayed session to run")
	}
	ctrl.Finish()
}

// stubProxyHandlers provides a session handler that writes a greeting and
// signals when a session has been served.
type stubProxyHandlers struct {
	sessionDone chan struct{}
}

// SessionHandler writes a greeting and signals session completion.
func (h *stubProxyHandlers) SessionHandler(session gliderssh.Session) {
	fmt.Fprintf(session, "relayed session\n")
	_ = session.Exit(0)
	close(h.sessionDone)
}

// newBlockingServer returns a terminating server whose session handler
// blocks until release is closed, keeping the relay's HandleConn (and thus
// its connection slot) held for the duration.
func newBlockingServer(c *tc.C, release <-chan struct{}) *gliderssh.Server {
	server := gliderssh.Server{
		ChannelHandlers: map[string]gliderssh.ChannelHandler{
			"session": gliderssh.DefaultSessionHandler,
		},
		Handler: func(session gliderssh.Session) {
			<-release
			_ = session.Exit(0)
		},
	}
	privateKey, err := test.InsecureKeyProfile()
	c.Assert(err, tc.ErrorIsNil)
	signer, err := gossh.NewSignerFromSigner(privateKey)
	c.Assert(err, tc.ErrorIsNil)
	server.AddHostKey(signer)
	return &server
}

func (s *relaySuite) TestResolveErrorWrittenToConn(c *tc.C) {
	ctrl := gomock.NewController(c)
	factory := NewMockTerminatingServerFactory(ctrl)
	factory.EXPECT().New(gomock.Any(), gomock.Any()).
		Return(nil, errors.New("no such destination"))

	conn, req := s.dialRelay(c, factory)

	// Write the upgrade request over a raw connection. After the 101
	// response the hijacked connection stays open on the server side,
	// which writes the resolve error as pre-banner text before closing.
	err := req.Write(conn)
	c.Assert(err, tc.ErrorIsNil)

	_ = conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	out, err := io.ReadAll(conn)
	c.Assert(err, tc.ErrorIsNil)
	// The internal error is written to the log, not the connection. The
	// client sees a generic message so target topology and service
	// errors are not leaked over the wire.
	c.Check(string(out), tc.Contains, "juju ssh relay: cannot reach destination")
	c.Check(string(out), tc.Not(tc.Contains), "no such destination")
	ctrl.Finish()
}

// TestConcurrentConnectionsCapped checks that a relay over the cap is
// rejected before the upgrade with an HTTP error, and that the
// destination is never resolved. A zero cap rejects every relay, so no
// blocking is needed to hold a slot.
func (s *relaySuite) TestConcurrentConnectionsCapped(c *tc.C) {
	ctrl := gomock.NewController(c)
	// The factory must not be called: rejection happens before the
	// upgrade and destination resolution.
	factory := NewMockTerminatingServerFactory(ctrl)

	handler := s.newHandlerMaxConns(c, factory, 0)

	token := newRelayToken(c, testModelUUID, string(permission.AdminAccess))
	destination := newMachineDestination(c, testModelUUID)
	r := httptest.NewRequest(http.MethodGet, "/ssh-relay/"+destination.String(), nil)
	r = r.WithContext(context.WithValue(r.Context(), RelayJWTKey{}, token))
	r.URL.RawQuery = ":virtualHostname=" + destination.String()

	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)

	c.Check(w.Code, tc.Equals, http.StatusServiceUnavailable)
	ctrl.Finish()
}

// TestRelayMaxConnectionsDrainAndReadmit exercises the live cap cycle over
// real connections: a relay holding a slot is counted, a second relay over
// the cap is rejected before the upgrade and never resolves its
// destination, and once the first session ends the slot drains so a later
// relay is re-admitted. This proves the deferred decrement resets the
// counter, which the zero-cap TestConcurrentConnectionsCapped cannot.
func (s *relaySuite) TestRelayMaxConnectionsDrainAndReadmit(c *tc.C) {
	ctrl := gomock.NewController(c)
	factory := NewMockTerminatingServerFactory(ctrl)

	// The factory is called once per admitted relay: the first slot holder
	// and, after the drain, the re-admitted relay. Each returns a server
	// whose session blocks on release, so the relay's HandleConn holds the
	// slot while we probe the cap. The over-cap relay is rejected before
	// resolution, so it never reaches the factory.
	destination := newMachineDestination(c, testModelUUID)
	release := make(chan struct{})
	factory.EXPECT().New(gomock.Any(), destination).
		DoAndReturn(func(context.Context, virtualhostname.Info) (*gliderssh.Server, error) {
			return newBlockingServer(c, release), nil
		}).Times(2)

	// Cap of one: a single relay fills the endpoint.
	handler := s.newHandlerMaxConns(c, factory, 1)

	// First relay: upgrade and start a session so the slot is held while
	// its server-side handler blocks on release.
	client := s.startBlockingRelay(c, handler)
	defer func() { _ = client.Close() }()
	s.checkRelayConnCount(c, handler, 1)

	// Second relay while the slot is held: rejected before the upgrade.
	over := s.serveRelayHandler(c, handler)
	c.Check(over.Code, tc.Equals, http.StatusServiceUnavailable)

	// Release the first session so the relay returns and the deferred
	// decrement drains the slot.
	close(release)
	_ = client.Close()
	s.checkRelayConnCount(c, handler, 0)

	// Third relay after the drain: re-admitted, proving the counter reset.
	// This session has nothing left to release, but it is admitted and its
	// handler exits immediately, so the connection completes on its own.
	admitted := s.startRelay(c, handler)
	_ = admitted.Close()

	ctrl.Finish()
}

// startBlockingRelay dials a relay against the handler, performs the HTTP
// upgrade and opens an SSH session. The session's server-side handler
// blocks, so the relay holds its slot until the caller releases it. The
// returned client keeps the connection open.
func (s *relaySuite) startBlockingRelay(c *tc.C, handler *RelayHandler) *gossh.Client {
	client := s.startRelay(c, handler)
	session, err := client.NewSession()
	c.Assert(err, tc.ErrorIsNil)
	// Start a session so the relay's HandleConn enters the blocking
	// handler. Do not wait for output: the handler blocks on release.
	c.Assert(session.Shell(), tc.ErrorIsNil)
	return client
}

// startRelay dials a relay against the handler, performs the HTTP upgrade
// and completes the SSH handshake, returning the connected client.
func (s *relaySuite) startRelay(c *tc.C, handler *RelayHandler) *gossh.Client {
	conn, req := s.dialRelayHandler(c, handler)
	upgraded, err := coresshproxy.PerformUpgrade(req, conn)
	c.Assert(err, tc.ErrorIsNil)

	sshConfig := &gossh.ClientConfig{
		HostKeyCallback: gossh.InsecureIgnoreHostKey(),
		Timeout:         5 * time.Second,
	}
	clientConn, chans, reqs, err := gossh.NewClientConn(upgraded, "relay", sshConfig)
	c.Assert(err, tc.ErrorIsNil)
	return gossh.NewClient(clientConn, chans, reqs)
}

// checkRelayConnCount polls until the handler's live connection count
// reaches want, so tests do not race the deferred increment or decrement.
func (s *relaySuite) checkRelayConnCount(c *tc.C, handler *RelayHandler, want int32) {
	for {
		if handler.concurrentConnections.Load() == want {
			return
		}
		select {
		case <-time.After(10 * time.Millisecond):
		case <-c.Context().Done():
			c.Fatalf("timed out waiting for relay connection count %d, got %d",
				want, handler.concurrentConnections.Load())
			return
		}
	}
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
		{"invalid permission rejected", testModelUUID, string(permission.SuperuserAccess), http.StatusForbidden},
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

// TestMalformedHostnameBadRequest checks that an unparseable destination
// hostname is rejected with a 400 before any authorization or upgrade.
func (s *relaySuite) TestMalformedHostnameBadRequest(c *tc.C) {
	ctrl := gomock.NewController(c)
	// The factory must not be called: the request is rejected on parse.
	factory := NewMockTerminatingServerFactory(ctrl)

	// Too many elements: an unambiguously unparseable hostname.
	const badHostname = "foo.bar.1.1." + testModelUUID + ".juju.local"
	r := httptest.NewRequest(http.MethodGet, "/ssh-relay/"+badHostname, nil)
	r.URL.RawQuery = ":virtualHostname=" + badHostname

	w := httptest.NewRecorder()
	s.newHandler(c, factory).ServeHTTP(w, r)

	c.Check(w.Code, tc.Equals, http.StatusBadRequest)
	c.Check(w.Body.String(), tc.Contains, "failed to parse destination hostname")
	ctrl.Finish()
}

// dialRelay starts a relay test server with the given factory, injects
// an admin JWT as the HTTP authentication layer would, and returns a raw
// connection to it along with an upgrade request ready to write.
func (s *relaySuite) dialRelay(c *tc.C, factory coresshproxy.TerminatingServerFactory) (net.Conn, *http.Request) {
	return s.dialRelayHandler(c, s.newHandler(c, factory))
}

// dialRelayHandler starts a relay test server serving the given handler,
// injects an admin JWT as the HTTP authentication layer would, and returns
// a raw connection to it along with an upgrade request ready to write. It
// serves the same handler instance on every request so the connection cap
// is shared across dials.
func (s *relaySuite) dialRelayHandler(c *tc.C, handler *RelayHandler) (net.Conn, *http.Request) {
	token := newRelayToken(c, testModelUUID, string(permission.AdminAccess))
	injectJWT := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := context.WithValue(r.Context(), RelayJWTKey{}, token)
		handler.ServeHTTP(w, r.WithContext(ctx))
	})
	server := httptest.NewServer(injectJWT)
	c.Cleanup(server.Close)

	conn, err := net.Dial("tcp", server.Listener.Addr().String())
	c.Assert(err, tc.ErrorIsNil)
	c.Cleanup(func() { _ = conn.Close() })

	destination := newMachineDestination(c, testModelUUID)
	req, err := http.NewRequest(http.MethodGet, server.URL+"/ssh-relay/"+destination.String(), nil)
	c.Assert(err, tc.ErrorIsNil)
	req.Header.Set("Connection", "Upgrade")
	req.Header.Set("Upgrade", coresshproxy.RelayUpgradeToken)
	req.URL.RawQuery = ":virtualHostname=" + destination.String()
	return conn, req
}

// serveRelayHandler dispatches an admin-JWT relay request against the given
// handler using a recorder, so the connection cap is observed on a handler
// whose slots may already be held by other in-flight relays.
func (s *relaySuite) serveRelayHandler(c *tc.C, handler *RelayHandler) *httptest.ResponseRecorder {
	token := newRelayToken(c, testModelUUID, string(permission.AdminAccess))
	destination := newMachineDestination(c, testModelUUID)
	r := httptest.NewRequest(http.MethodGet, "/ssh-relay/"+destination.String(), nil)
	r = r.WithContext(context.WithValue(r.Context(), RelayJWTKey{}, token))
	r.URL.RawQuery = ":virtualHostname=" + destination.String()

	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)
	return w
}

func (s *relaySuite) newHandler(c *tc.C, factory coresshproxy.TerminatingServerFactory) *RelayHandler {
	return s.newHandlerMaxConns(c, factory, 10)
}

func (s *relaySuite) newHandlerMaxConns(c *tc.C, factory coresshproxy.TerminatingServerFactory, maxConns int) *RelayHandler {
	handler, err := NewRelayHandler(RelayHandlerConfig{
		Logger:                   loggertesting.WrapCheckLog(c),
		ServerFactory:            factory,
		MaxConcurrentConnections: func() int { return maxConns },
	})
	c.Assert(err, tc.ErrorIsNil)
	return handler
}

// serveRelay dispatches a relay request and returns the response recorder.
// An empty access produces no JWT, testing the missing-token path.
func (s *relaySuite) serveRelay(c *tc.C, factory coresshproxy.TerminatingServerFactory, modelUUID, access string) *httptest.ResponseRecorder {
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
