// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package sshproxy

import (
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/juju/tc"

	coresshproxy "github.com/juju/juju/core/sshproxy"
)

type HandlerSuite struct{}

func TestHandlerSuite(t *testing.T) {
	tc.Run(t, &HandlerSuite{})
}

// hijackResult captures what happened inside a hijack-under-test handler,
// since hijack itself must run inside a real HTTP handler to have access to
// a http.Hijacker.
type hijackResult struct {
	conn net.Conn
	err  error
}

// serveHijack starts an httptest server whose single handler calls hijack
// with the given token and reports the result on the returned channel. The
// caller is responsible for closing the server and any returned connection.
func serveHijack(token string) (*httptest.Server, <-chan hijackResult) {
	results := make(chan hijackResult, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := hijack(w, r, token)
		results <- hijackResult{conn: conn, err: err}
	}))
	return srv, results
}

// dial opens a raw TCP connection to the server and writes an upgrade
// request with the given Connection and Upgrade header values.
func dial(c *tc.C, addr string, connectionHeader, upgradeHeader string) net.Conn {
	conn, err := net.Dial("tcp", addr)
	c.Assert(err, tc.ErrorIsNil)
	c.Cleanup(func() { _ = conn.Close() })

	req, err := http.NewRequest(http.MethodGet, "http://"+addr+"/", nil)
	c.Assert(err, tc.ErrorIsNil)
	if connectionHeader != "" {
		req.Header.Set("Connection", connectionHeader)
	}
	if upgradeHeader != "" {
		req.Header.Set("Upgrade", upgradeHeader)
	}
	c.Assert(req.Write(conn), tc.ErrorIsNil)
	return conn
}

func (s *HandlerSuite) TestHijackAcceptsExactUpgradeConnection(c *tc.C) {
	srv, results := serveHijack(coresshproxy.TunnelUpgradeToken)
	defer srv.Close()

	client := dial(c, srv.Listener.Addr().String(), "Upgrade", coresshproxy.TunnelUpgradeToken)
	defer client.Close()

	res := waitHijackResult(c, results)
	defer func() {
		if res.conn != nil {
			_ = res.conn.Close()
		}
	}()
	c.Assert(res.err, tc.ErrorIsNil)
	c.Assert(res.conn, tc.NotNil)
}

func (s *HandlerSuite) TestHijackAcceptsUpgradeAmongMultipleConnectionTokens(c *tc.C) {
	srv, results := serveHijack(coresshproxy.TunnelUpgradeToken)
	defer srv.Close()

	// A proxy or intermediary may combine tokens, e.g. "keep-alive, Upgrade".
	client := dial(c, srv.Listener.Addr().String(), "keep-alive, Upgrade", coresshproxy.TunnelUpgradeToken)
	defer client.Close()

	res := waitHijackResult(c, results)
	defer func() {
		if res.conn != nil {
			_ = res.conn.Close()
		}
	}()
	c.Assert(res.err, tc.ErrorIsNil)
	c.Assert(res.conn, tc.NotNil)
}

func (s *HandlerSuite) TestHijackRejectsMissingUpgradeToken(c *tc.C) {
	srv, results := serveHijack(coresshproxy.TunnelUpgradeToken)
	defer srv.Close()

	// "Connection: keep-alive" alone does not request an upgrade at all.
	client := dial(c, srv.Listener.Addr().String(), "keep-alive", coresshproxy.TunnelUpgradeToken)
	defer client.Close()

	res := waitHijackResult(c, results)
	c.Assert(res.conn, tc.IsNil)
	c.Assert(res.err, tc.ErrorMatches, "expected Upgrade:.*")
}

func (s *HandlerSuite) TestHijackRejectsWrongUpgradeValue(c *tc.C) {
	srv, results := serveHijack(coresshproxy.TunnelUpgradeToken)
	defer srv.Close()

	client := dial(c, srv.Listener.Addr().String(), "Upgrade", "some-other-protocol")
	defer client.Close()

	res := waitHijackResult(c, results)
	c.Assert(res.conn, tc.IsNil)
	c.Assert(res.err, tc.ErrorMatches, "expected Upgrade:.*")
}

func waitHijackResult(c *tc.C, results <-chan hijackResult) hijackResult {
	select {
	case res := <-results:
		return res
	case <-time.After(longWait):
		c.Fatalf("timed out waiting for hijack result")
		return hijackResult{}
	}
}

const longWait = 5 * time.Second
