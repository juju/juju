// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package ssh

import (
	"io"
	"net"
	"strings"
	"testing"

	"github.com/juju/tc"
)

type preBannerSuite struct{}

func TestPreBannerSuite(t *testing.T) {
	tc.Run(t, &preBannerSuite{})
}

// readPreBanner reads from conn until the writing side signals it is
// done and closes its end, which makes ReadAll return.
func readPreBanner(c *tc.C, conn net.Conn, done <-chan error) string {
	type result struct {
		out string
		err error
	}
	resultCh := make(chan result, 1)
	go func() {
		out, err := io.ReadAll(conn)
		resultCh <- result{string(out), err}
	}()
	c.Assert(<-done, tc.ErrorIsNil)
	res := <-resultCh
	c.Assert(res.err, tc.ErrorIsNil)
	return res.out
}

// writeAndRead runs WritePreBannerError on one end of a pipe and
// returns what the other end reads.
func writeAndRead(c *tc.C, msg string) string {
	client, server := net.Pipe()
	defer func() { _ = client.Close() }()

	done := make(chan error, 1)
	go func() {
		done <- WritePreBannerError(server, msg)
		_ = server.Close()
	}()
	return readPreBanner(c, client, done)
}

func (*preBannerSuite) TestWritePreBannerError(c *tc.C) {
	for _, test := range []struct {
		name string
		msg  string
		out  string
	}{
		{"basic", "no such destination", "no such destination\r\n"},
		{"flattens newlines", "line one\nline two", "line one line two\r\n"},
		{"caps length", strings.Repeat("x", 300), strings.Repeat("x", 253) + "\r\n"},
		{"caps on rune boundary", strings.Repeat("é", 128), strings.Repeat("é", 126) + "\r\n"},
	} {
		c.Run(test.name, func(t *testing.T) {
			tc.Check(t, writeAndRead(c, test.msg), tc.Equals, test.out)
		})
	}
}

func (*preBannerSuite) TestWritePreBannerErrorWriteFailure(c *tc.C) {
	client, server := net.Pipe()
	_ = client.Close()
	_ = server.Close()

	err := WritePreBannerError(server, "message")
	c.Check(err, tc.Not(tc.ErrorIsNil))
}
