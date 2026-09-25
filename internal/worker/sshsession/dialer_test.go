// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package sshsession

import (
	"context"
	"crypto/tls"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/bmizerany/pat"
	"github.com/juju/names/v6"
	"github.com/juju/tc"

	"github.com/juju/juju/api"
	"github.com/juju/juju/core/sshproxy"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/testhelpers"
	coretesting "github.com/juju/juju/internal/testing"
)

type ConnectionDialerSuite struct {
	testhelpers.IsolationSuite
}

func TestConnectionDialerSuite(t *testing.T) {
	tc.Run(t, &ConnectionDialerSuite{})
}

// upgradeTunnelServer is a minimal TLS server exercising a real HTTP
// upgrade handshake, so DialController/upgrade is tested against real TLS
// and HTTP rather than a mock. The full server handler is tested
// separately in apiserver/internal/handlers/sshproxy.
type upgradeTunnelServer struct {
	listener net.Listener
	// accepted receives the hijacked server-side connection for each
	// accepted request.
	accepted chan net.Conn
}

func startUpgradeTunnelServer(c *tc.C) *upgradeTunnelServer {
	cert, err := tls.X509KeyPair([]byte(coretesting.ServerCert), []byte(coretesting.ServerKey))
	c.Assert(err, tc.ErrorIsNil)

	rawListener, err := net.Listen("tcp", "127.0.0.1:0")
	c.Assert(err, tc.ErrorIsNil)
	tlsListener := tls.NewListener(rawListener, &tls.Config{
		Certificates: []tls.Certificate{cert},
	})

	accepted := make(chan net.Conn, 1)
	srv := &upgradeTunnelServer{
		listener: tlsListener,
		accepted: accepted,
	}

	// Route through pat, the same muxer the real apiserver uses, so the
	// dialer's request path (/model/:modeluuid/ssh-tunnel/:tunnelID)
	// resolves the way it does in production.
	mux := pat.New()
	mux.Get("/model/:modeluuid/ssh-tunnel/:tunnelID", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn := upgradeConn(c, w, r)
		if conn != nil {
			accepted <- conn
		}
	}))

	httpServer := &http.Server{Handler: mux}
	go func() { _ = httpServer.Serve(tlsListener) }()
	c.Cleanup(func() {
		_ = httpServer.Close()
	})
	return srv
}

// upgradeConn performs the server-side half of the HTTP upgrade handshake
// (validate headers, respond 101, hijack) directly against net/http,
// mirroring the production hijack helper without depending on it.
func upgradeConn(c *tc.C, w http.ResponseWriter, r *http.Request) net.Conn {
	if r.Header.Get("Upgrade") != sshproxy.TunnelUpgradeToken {
		http.Error(w, "invalid upgrade request", http.StatusBadRequest)
		return nil
	}
	hijacker, ok := w.(http.Hijacker)
	if !ok {
		http.Error(w, "connection cannot be upgraded", http.StatusInternalServerError)
		return nil
	}
	w.Header().Set("Connection", "Upgrade")
	w.Header().Set("Upgrade", sshproxy.TunnelUpgradeToken)
	w.WriteHeader(http.StatusSwitchingProtocols)

	conn, _, err := hijacker.Hijack()
	c.Check(err, tc.ErrorIsNil)
	return conn
}

func (s *ConnectionDialerSuite) TestDialControllerPerformsRealTLSUpgrade(c *tc.C) {
	srv := startUpgradeTunnelServer(c)

	apiInfo := &api.Info{
		Addrs:  []string{srv.listener.Addr().String()},
		CACert: coretesting.CACert,
		Tag:    names.NewMachineTag("0"),
	}
	dialer := newConnectionDialer(loggertesting.WrapCheckLog(c), apiInfo)

	ctx, cancel := context.WithTimeout(c.Context(), 5*time.Second)
	defer cancel()
	conn, err := dialer.DialController(ctx, srv.listener.Addr().String(), "model-uuid", "tunnel-0")
	c.Assert(err, tc.ErrorIsNil)
	defer conn.Close()

	// The server side of the same connection was hijacked and reported by
	// the test upgrade handler: writing on it must be observable on the
	// client side, proving this is a genuine raw connection past the HTTP
	// layer.
	var serverConn net.Conn
	select {
	case serverConn = <-srv.accepted:
	case <-time.After(5 * time.Second):
		c.Fatalf("server never accepted the upgrade")
	}
	defer serverConn.Close()

	_, err = serverConn.Write([]byte("hello"))
	c.Assert(err, tc.ErrorIsNil)

	buf := make([]byte, 5)
	c.Assert(conn.SetReadDeadline(time.Now().Add(5*time.Second)), tc.ErrorIsNil)
	n, err := conn.Read(buf)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(string(buf[:n]), tc.Equals, "hello")
}

func (s *ConnectionDialerSuite) TestDialControllerFailsOnUntrustedCert(c *tc.C) {
	srv := startUpgradeTunnelServer(c)

	// A different CA than the one the server's certificate was signed
	// with: the dial must fail during the TLS handshake.
	apiInfo := &api.Info{
		Addrs:  []string{srv.listener.Addr().String()},
		CACert: coretesting.OtherCACert,
		Tag:    names.NewMachineTag("0"),
	}
	dialer := newConnectionDialer(loggertesting.WrapCheckLog(c), apiInfo)

	ctx, cancel := context.WithTimeout(c.Context(), 5*time.Second)
	defer cancel()
	_, err := dialer.DialController(ctx, srv.listener.Addr().String(), "model-uuid", "tunnel-0")
	c.Assert(err, tc.ErrorMatches, "dialing controller.*")
}

// noHalfCloseConn is a net.Conn that does not implement CloseWrite, unlike
// every real transport this dialer uses (TCP, TLS).
type noHalfCloseConn struct {
	net.Conn
}

func (s *ConnectionDialerSuite) TestAsHalfCloseConnRejectsUnsupportedConn(c *tc.C) {
	client, server := net.Pipe()
	defer client.Close()
	defer server.Close()

	_, err := asHalfCloseConn(&noHalfCloseConn{Conn: client})
	c.Assert(err, tc.ErrorMatches, ".*does not support half-close")
}
