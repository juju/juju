// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package ssh

import (
	"io"
	"net"
	"strings"
	"testing"
	"unicode/utf8"

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

func (*preBannerSuite) TestWritePreBannerError(c *tc.C) {
	client, server := net.Pipe()
	defer func() { _ = client.Close() }()

	done := make(chan error, 1)
	go func() {
		done <- WritePreBannerError(server, "no such destination")
		_ = server.Close()
	}()

	out := readPreBanner(c, client, done)
	c.Check(out, tc.Equals, "no such destination\r\n")
}

func (*preBannerSuite) TestWritePreBannerErrorFlattensNewlines(c *tc.C) {
	client, server := net.Pipe()
	defer func() { _ = client.Close() }()

	done := make(chan error, 1)
	go func() {
		done <- WritePreBannerError(server, "line one\nline two\nline three")
		_ = server.Close()
	}()

	out := readPreBanner(c, client, done)
	c.Check(out, tc.Equals, "line one line two line three\r\n")
}

func (*preBannerSuite) TestWritePreBannerErrorCapsLength(c *tc.C) {
	client, server := net.Pipe()
	defer func() { _ = client.Close() }()

	msg := strings.Repeat("x", 300)

	done := make(chan error, 1)
	go func() {
		done <- WritePreBannerError(server, msg)
		_ = server.Close()
	}()

	out := readPreBanner(c, client, done)
	c.Check(out, tc.Equals, strings.Repeat("x", 80)+"\r\n")
}

func (*preBannerSuite) TestWritePreBannerErrorCapsLengthOnRuneBoundary(c *tc.C) {
	client, server := net.Pipe()
	defer func() { _ = client.Close() }()

	// é is two bytes in UTF-8. 41 runes are 82 bytes, so the byte cap
	// at 80 would split the final rune; the backup to a rune boundary
	// must drop it and emit 40 valid runes.
	msg := strings.Repeat("é", 41)

	done := make(chan error, 1)
	go func() {
		done <- WritePreBannerError(server, msg)
		_ = server.Close()
	}()

	out := readPreBanner(c, client, done)
	c.Check(out, tc.Equals, strings.Repeat("é", 40)+"\r\n")
	c.Check(utf8.ValidString(out), tc.IsTrue)
}

func (*preBannerSuite) TestWritePreBannerErrorWriteFailure(c *tc.C) {
	client, server := net.Pipe()
	_ = client.Close()
	_ = server.Close()

	err := WritePreBannerError(server, "message")
	c.Check(err, tc.Not(tc.ErrorIsNil))
}
