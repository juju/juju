// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package sshstorage_test

import (
	"bytes"
	"errors"
	"io"

	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/environs/sshstorage"
)

type wrapWriterSuite struct{}

var _ = gc.Suite(&wrapWriterSuite{})

func (*wrapWriterSuite) TestLineWrapWriterBadLength(c *gc.C) {
	var buf bytes.Buffer
	c.Assert(func() { sshstorage.NewLineWrapWriter(&buf, 0) }, gc.PanicMatches, "lineWrapWriter with line length <= 0")
	c.Assert(func() { sshstorage.NewLineWrapWriter(&buf, -1) }, gc.PanicMatches, "lineWrapWriter with line length <= 0")
}

func (*wrapWriterSuite) TestLineWrapWriter(c *gc.C) {
	type test struct {
		input      []string
		lineLength int
		expected   string
	}
	tests := []test{{
		input:      []string{""},
		lineLength: 1,
		expected:   "",
	}, {
		input:      []string{"hi!"},
		lineLength: 1,
		expected:   "h\ni\n!\n",
	}, {
		input:      []string{"hi!"},
		lineLength: 2,
		// Note: no trailing newline.
		expected: "hi\n!",
	}, {
		input:      []string{"", "h", "i!"},
		lineLength: 2,
		expected:   "hi\n!",
	}, {
		input:      []string{"", "h", "i!"},
		lineLength: 2,
		expected:   "hi\n!",
	}, {
		input:      []string{"hi", "!!"},
		lineLength: 2,
		expected:   "hi\n!!\n",
	}, {
		input:      []string{"hi", "!", "!"},
		lineLength: 2,
		expected:   "hi\n!!\n",
	}, {
		input:      []string{"h", "i", "!!"},
		lineLength: 2,
		expected:   "hi\n!!\n",
	}}
	for i, t := range tests {
		c.Logf("test %d: %q, line length %d", i, t.input, t.lineLength)
		var buf bytes.Buffer
		w := sshstorage.NewLineWrapWriter(&buf, t.lineLength)
		c.Assert(w, gc.NotNil)
		for _, input := range t.input {
			n, err := w.Write([]byte(input))
			c.Assert(err, gc.IsNil)
			c.Assert(n, gc.Equals, len(input))
		}
		c.Assert(buf.String(), gc.Equals, t.expected)
	}
}

type limitedWriter struct {
	io.Writer
	remaining int
}

var writeLimited = errors.New("write limited")

func (w *limitedWriter) Write(buf []byte) (int, error) {
	inputlen := len(buf)
	if len(buf) > w.remaining {
		buf = buf[:w.remaining]
	}
	n, err := w.Writer.Write(buf)
	w.remaining -= n
	if n < inputlen && err == nil {
		err = writeLimited
	}
	return n, err
}

func (*wrapWriterSuite) TestLineWrapWriterErrors(c *gc.C) {
	// Note: after an error is returned, all bets are off.
	// In the only place we use this code, we bail out immediately.
	const lineLength = 3
	type test struct {
		input   string
		output  string
		limit   int
		written int
		err     error
	}
	tests := []test{{
		input:   "abc",
		output:  "abc",
		limit:   3, // "\n" will be limited
		written: 3,
		err:     writeLimited,
	}, {
		input:   "abc",
		output:  "abc\n",
		limit:   4,
		written: 3, // 3/3 bytes of input
	}, {
		input:   "abc",
		output:  "ab",
		limit:   2,
		written: 2, // 2/3 bytes of input
		err:     writeLimited,
	}, {
		input:   "abc!",
		output:  "abc\n",
		limit:   4,
		written: 3, // 3/4 bytes of input
		err:     writeLimited,
	}}
	for i, t := range tests {
		c.Logf("test %d: %q, limit %d", i, t.input, t.limit)
		var buf bytes.Buffer
		wrapWriter := &limitedWriter{&buf, t.limit}
		w := sshstorage.NewLineWrapWriter(wrapWriter, lineLength)
		c.Assert(w, gc.NotNil)
		n, err := w.Write([]byte(t.input))
		c.Assert(n, gc.Equals, t.written)
		c.Assert(buf.String(), gc.Equals, t.output)
		c.Assert(err, gc.Equals, t.err)
	}
}
