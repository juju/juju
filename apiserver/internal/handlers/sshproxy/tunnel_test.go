// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package sshproxy

import (
	"bufio"
	"context"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/canonical/gomock/gomock"
	"github.com/juju/tc"

	coresshproxy "github.com/juju/juju/core/sshproxy"
	"github.com/juju/juju/internal/errors"
	loggertesting "github.com/juju/juju/internal/logger/testing"
)

type TunnelHandlerSuite struct{}

func TestTunnelHandlerSuite(t *testing.T) {
	tc.Run(t, &TunnelHandlerSuite{})
}

// ctxConfig configures the request context values the apiserver's real
// wrapper would normally inject.
type ctxConfig struct {
	machineName    string
	hasMachineName bool
}

func wrapWithContext(h http.Handler, cfg ctxConfig) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		if cfg.hasMachineName {
			ctx = context.WithValue(ctx, AuthenticatedMachineNameKey{}, cfg.machineName)
		}
		h.ServeHTTP(w, r.WithContext(ctx))
	})
}

// withFinishedSignal wraps h so the returned channel is closed once
// ServeHTTP returns, letting tests detect completion of the handler.
func withFinishedSignal(h http.Handler) (http.Handler, <-chan struct{}) {
	finished := make(chan struct{})
	wrapped := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer close(finished)
		h.ServeHTTP(w, r)
	})
	return wrapped, finished
}

// newTunnelTestServer builds an httptest server around handler with the
// given context values injected, and returns a channel that is closed once
// the handler returns from a request.
func newTunnelTestServer(c *tc.C, handler *TunnelHandler, cfg ctxConfig) (*httptest.Server, <-chan struct{}) {
	tracked, finished := withFinishedSignal(handler)
	srv := httptest.NewServer(wrapWithContext(tracked, cfg))
	c.Cleanup(srv.Close)
	return srv, finished
}

func newTunnelHandler(c *tc.C, tracker TunnelTracker) *TunnelHandler {
	h, err := NewTunnelHandler(TunnelHandlerConfig{
		Logger:  loggertesting.WrapCheckLog(c),
		Tracker: tracker,
	})
	c.Assert(err, tc.ErrorIsNil)
	return h
}

func (s *TunnelHandlerSuite) TestValidateRejectsMissingDependencies(c *tc.C) {
	_, err := NewTunnelHandler(TunnelHandlerConfig{})
	c.Assert(err, tc.ErrorMatches, ".*nil Logger.*")

	_, err = NewTunnelHandler(TunnelHandlerConfig{
		Logger: loggertesting.WrapCheckLog(c),
	})
	c.Assert(err, tc.ErrorMatches, ".*nil Tracker.*")
}

func (s *TunnelHandlerSuite) TestServeHTTPMissingAuthenticatedMachine(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()
	tracker := NewMockTunnelTracker(ctrl)

	handler := newTunnelHandler(c, tracker)
	srv, _ := newTunnelTestServer(c, handler, ctxConfig{})

	resp, err := srv.Client().Get(srv.URL + "/?:tunnelID=tunnel-0")
	c.Assert(err, tc.ErrorIsNil)
	defer resp.Body.Close()
	c.Check(resp.StatusCode, tc.Equals, http.StatusUnauthorized)
}

func (s *TunnelHandlerSuite) TestServeHTTPMissingTunnelID(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()
	tracker := NewMockTunnelTracker(ctrl)

	handler := newTunnelHandler(c, tracker)
	srv, _ := newTunnelTestServer(c, handler, ctxConfig{machineName: "0", hasMachineName: true})

	resp, err := srv.Client().Get(srv.URL + "/")
	c.Assert(err, tc.ErrorIsNil)
	defer resp.Body.Close()
	c.Check(resp.StatusCode, tc.Equals, http.StatusBadRequest)
}

// dialTunnel opens a raw TCP connection to addr and writes an upgrade
// request for tunnelID, returning the connection and the parsed response
// head. The caller owns the connection afterwards.
func dialTunnel(c *tc.C, addr, tunnelID string) (net.Conn, *http.Response) {
	conn, err := net.Dial("tcp", addr)
	c.Assert(err, tc.ErrorIsNil)
	c.Cleanup(func() { _ = conn.Close() })

	req, err := http.NewRequest(http.MethodGet, "http://"+addr+"/", nil)
	c.Assert(err, tc.ErrorIsNil)
	req.URL.RawQuery = ":tunnelID=" + tunnelID
	req.Header.Set("Connection", "Upgrade")
	req.Header.Set("Upgrade", coresshproxy.TunnelUpgradeToken)
	c.Assert(req.Write(conn), tc.ErrorIsNil)

	resp, err := http.ReadResponse(bufio.NewReader(conn), req)
	c.Assert(err, tc.ErrorIsNil)
	return conn, resp
}

func (s *TunnelHandlerSuite) TestServeHTTPPushTunnelErrorClosesConn(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()
	tracker := NewMockTunnelTracker(ctrl)
	tracker.EXPECT().PushTunnel(gomock.Any(), "tunnel-0", "0", gomock.Any()).Return(nil, errors.New("push failed"))

	handler := newTunnelHandler(c, tracker)
	srv, finished := newTunnelTestServer(c, handler, ctxConfig{machineName: "0", hasMachineName: true})

	conn, resp := dialTunnel(c, srv.Listener.Addr().String(), "tunnel-0")
	defer resp.Body.Close()
	c.Assert(resp.StatusCode, tc.Equals, http.StatusSwitchingProtocols)

	waitFinished(c, finished)

	// The handler closes conn after a failed push: a read now observes
	// a closed connection error rather than blocking.
	_ = conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	buf := make([]byte, 1)
	_, err := conn.Read(buf)
	c.Check(err, tc.ErrorIs, io.EOF)
}

func (s *TunnelHandlerSuite) TestServeHTTPBlocksUntilTunnelDone(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()
	tracker := NewMockTunnelTracker(ctrl)

	pushed := make(chan net.Conn, 1)
	done := make(chan struct{})
	tracker.EXPECT().PushTunnel(gomock.Any(), "tunnel-0", "0", gomock.Any()).DoAndReturn(
		func(_ context.Context, _ string, _ string, conn net.Conn) (<-chan struct{}, error) {
			pushed <- conn
			return done, nil
		})

	handler := newTunnelHandler(c, tracker)
	srv, finished := newTunnelTestServer(c, handler, ctxConfig{machineName: "0", hasMachineName: true})

	_, resp := dialTunnel(c, srv.Listener.Addr().String(), "tunnel-0")
	defer resp.Body.Close()
	c.Assert(resp.StatusCode, tc.Equals, http.StatusSwitchingProtocols)

	// The connection was pushed to the tracker; the handler must still be
	// blocked in the done/dying select.
	select {
	case <-pushed:
	case <-time.After(5 * time.Second):
		c.Fatalf("timed out waiting for tunnel to be pushed")
	}
	select {
	case <-finished:
		c.Fatalf("handler returned before tunnel done fired")
	case <-time.After(50 * time.Millisecond):
	}

	close(done)
	waitFinished(c, finished)
}

func (s *TunnelHandlerSuite) TestServeHTTPClosesConnOnKill(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()
	tracker := NewMockTunnelTracker(ctrl)

	pushed := make(chan net.Conn, 1)
	tracker.EXPECT().PushTunnel(gomock.Any(), "tunnel-0", "0", gomock.Any()).DoAndReturn(
		func(_ context.Context, _ string, _ string, conn net.Conn) (<-chan struct{}, error) {
			pushed <- conn
			return make(chan struct{}), nil
		})

	handler := newTunnelHandler(c, tracker)
	srv, finished := newTunnelTestServer(c, handler, ctxConfig{
		machineName:    "0",
		hasMachineName: true,
	})

	conn, resp := dialTunnel(c, srv.Listener.Addr().String(), "tunnel-0")
	defer resp.Body.Close()
	c.Assert(resp.StatusCode, tc.Equals, http.StatusSwitchingProtocols)

	select {
	case <-pushed:
	case <-time.After(5 * time.Second):
		c.Fatalf("timed out waiting for tunnel to be pushed")
	}

	// Killing the handler must close the hijacked connection and let
	// ServeHTTP return.
	handler.Kill()
	waitFinished(c, finished)
	c.Assert(handler.Wait(), tc.ErrorIsNil)

	_ = conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	buf := make([]byte, 1)
	_, err := conn.Read(buf)
	c.Check(err, tc.NotNil)
}

func (s *TunnelHandlerSuite) TestServeHTTPRejectsWrongMachine(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()
	tracker := NewMockTunnelTracker(ctrl)
	// The tracker enforces the binding. A mismatch surfaces as a push
	// error, which closes the connection.
	tracker.EXPECT().PushTunnel(gomock.Any(), "tunnel-0", "1", gomock.Any()).Return(nil, errors.New("tunnel \"tunnel-0\" does not belong to machine \"1\""))

	handler := newTunnelHandler(c, tracker)
	srv, finished := newTunnelTestServer(c, handler, ctxConfig{machineName: "1", hasMachineName: true})

	conn, resp := dialTunnel(c, srv.Listener.Addr().String(), "tunnel-0")
	defer resp.Body.Close()
	c.Assert(resp.StatusCode, tc.Equals, http.StatusSwitchingProtocols)

	waitFinished(c, finished)

	_ = conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	buf := make([]byte, 1)
	_, err := conn.Read(buf)
	c.Check(err, tc.ErrorIs, io.EOF)
}

func waitFinished(c *tc.C, finished <-chan struct{}) {
	select {
	case <-finished:
	case <-time.After(5 * time.Second):
		c.Fatalf("timed out waiting for handler to return")
	}
}
