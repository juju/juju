// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package sshproxy

import (
	"bufio"
	"io"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/juju/tc"
)

type UpgradeSuite struct{}

func TestUpgradeSuite(t *testing.T) {
	tc.Run(t, &UpgradeSuite{})
}

// newUpgradeRequest builds the request PerformUpgrade would send for a
// tunnel dial.
func newUpgradeRequest(c *tc.C) *http.Request {
	req, err := http.NewRequest(http.MethodGet, "https://controller/model/m/ssh-tunnel/tunnel-0", nil)
	c.Assert(err, tc.ErrorIsNil)
	req.Header.Set("Connection", "Upgrade")
	req.Header.Set("Upgrade", TunnelUpgradeToken)
	return req
}

// serveRaw starts a goroutine that reads (and discards) the request line
// and headers off the server side of conn, then writes raw bytes back. It
// does not close serverSide: net.Pipe has no internal buffering, so closing
// immediately after the write would race with the client still reading
// those same bytes. The caller owns serverSide's lifetime.
func serveRaw(c *tc.C, serverSide net.Conn, raw []byte) {
	go func() {
		reader := bufio.NewReader(serverSide)
		req, err := http.ReadRequest(reader)
		if err != nil {
			return
		}
		_ = req.Body.Close()
		_, _ = serverSide.Write(raw)
	}()
}

func (s *UpgradeSuite) TestPerformUpgradeSuccess(c *tc.C) {
	client, server := net.Pipe()
	defer client.Close()
	defer server.Close()
	serveRaw(c, server, []byte("HTTP/1.1 101 Switching Protocols\r\nConnection: Upgrade\r\nUpgrade: "+TunnelUpgradeToken+"\r\n\r\n"))

	conn, err := PerformUpgrade(newUpgradeRequest(c), client)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(conn, tc.NotNil)

	// No buffered bytes past the response head, so the original connection
	// is returned unwrapped.
	_, ok := conn.(*prefixConn)
	c.Check(ok, tc.IsFalse)
}

func (s *UpgradeSuite) TestPerformUpgradeRejectedStatus(c *tc.C) {
	client, server := net.Pipe()
	defer client.Close()
	defer server.Close()
	serveRaw(c, server, []byte("HTTP/1.1 403 Forbidden\r\nContent-Length: 13\r\n\r\nnot permitted"))

	conn, err := PerformUpgrade(newUpgradeRequest(c), client)
	c.Assert(conn, tc.IsNil)
	c.Assert(err, tc.ErrorMatches, `upgrade rejected: 403 Forbidden: not permitted`)
}

func (s *UpgradeSuite) TestPerformUpgradePreservesBufferedBytes(c *tc.C) {
	client, server := net.Pipe()
	defer client.Close()
	defer server.Close()
	// The server writes bytes immediately after the response head (e.g. an
	// SSH banner); PerformUpgrade must not lose them even though they were
	// buffered while reading the HTTP response.
	serveRaw(c, server, []byte(
		"HTTP/1.1 101 Switching Protocols\r\nConnection: Upgrade\r\nUpgrade: "+TunnelUpgradeToken+"\r\n\r\n"+
			"SSH-2.0-banner\r\n"))

	conn, err := PerformUpgrade(newUpgradeRequest(c), client)
	c.Assert(err, tc.ErrorIsNil)

	buf := make([]byte, len("SSH-2.0-banner\r\n"))
	n, err := io.ReadFull(withDeadline(c, conn), buf)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(string(buf[:n]), tc.Equals, "SSH-2.0-banner\r\n")
}

func (s *UpgradeSuite) TestPerformUpgradeWriteError(c *tc.C) {
	client, server := net.Pipe()
	_ = server.Close()
	_ = client.Close()

	_, err := PerformUpgrade(newUpgradeRequest(c), client)
	c.Assert(err, tc.ErrorMatches, "writing upgrade request:.*")
}

// withDeadline sets a read deadline on conn (if supported) so a stuck test
// fails fast rather than hanging.
func withDeadline(c *tc.C, conn net.Conn) net.Conn {
	c.Assert(conn.SetReadDeadline(time.Now().Add(5*time.Second)), tc.ErrorIsNil)
	return conn
}

func (s *UpgradeSuite) TestPrefixConnReadDrainsPrefixThenUnderlying(c *tc.C) {
	client, server := net.Pipe()
	defer client.Close()
	defer server.Close()

	pc := &prefixConn{Conn: client, prefix: []byte("abc")}

	buf := make([]byte, 2)
	n, err := pc.Read(buf)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(string(buf[:n]), tc.Equals, "ab")

	n, err = pc.Read(buf)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(string(buf[:n]), tc.Equals, "c")

	// The prefix is now drained; further reads go to the underlying conn.
	written := make(chan struct{})
	go func() {
		defer close(written)
		_, _ = server.Write([]byte("xy"))
	}()
	buf = make([]byte, 2)
	_ = pc.SetReadDeadline(time.Now().Add(5 * time.Second))
	n, err = pc.Read(buf)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(string(buf[:n]), tc.Equals, "xy")
	<-written
}

// halfCloseConn wraps net.Conn with a CloseWrite method, since net.Pipe
// connections don't support half-close natively.
type halfCloseConn struct {
	net.Conn
	closedWrite bool
}

func (c *halfCloseConn) CloseWrite() error {
	c.closedWrite = true
	return nil
}

func (s *UpgradeSuite) TestPrefixConnCloseWriteDelegates(c *tc.C) {
	client, server := net.Pipe()
	defer client.Close()
	defer server.Close()

	hc := &halfCloseConn{Conn: client}
	pc := &prefixConn{Conn: hc}

	c.Assert(pc.CloseWrite(), tc.ErrorIsNil)
	c.Check(hc.closedWrite, tc.IsTrue)
}

func (s *UpgradeSuite) TestPrefixConnCloseWriteNoop(c *tc.C) {
	client, server := net.Pipe()
	defer client.Close()
	defer server.Close()

	// net.Pipe's Conn does not implement CloseWrite; CloseWrite must be a
	// harmless no-op rather than panicking or erroring.
	pc := &prefixConn{Conn: client}
	c.Assert(pc.CloseWrite(), tc.ErrorIsNil)
}
