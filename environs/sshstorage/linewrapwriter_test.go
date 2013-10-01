// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package sshstorage_test

import (
	"bytes"

	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/environs/sshstorage"
)

type wrapWriterSuite struct{}

var _ = gc.Suite(&wrapWriterSuite{})

func (*wrapWriterSuite) TestLineWrapWriterBadLength(c *gc.C) {
	var buf bytes.Buffer
	w, err := sshstorage.NewLineWrapWriter(&buf, 0)
	c.Assert(err, gc.ErrorMatches, "line length 0 <= 0")
	c.Assert(w, gc.IsNil)
	w, err = sshstorage.NewLineWrapWriter(&buf, -1)
	c.Assert(err, gc.ErrorMatches, "line length -1 <= 0")
}

func (*wrapWriterSuite) TestLineWrapWriter(c *gc.C) {
	type test struct {
		input      string
		lineLength int
		expected   string
	}
	tests := []test{{
		input:      "",
		lineLength: 1,
		expected:   "",
	}, {
		input:      "hi!",
		lineLength: 1,
		expected:   "h\ni\n!\n",
	}, {
		input:      "hi!",
		lineLength: 2,
		// Note: no trailing newline.
		expected: "hi\n!",
	}}
	for i, t := range tests {
		c.Logf("test %d: %q, line length %d", i, t.input, t.lineLength)
		var buf bytes.Buffer
		w, err := sshstorage.NewLineWrapWriter(&buf, t.lineLength)
		c.Assert(err, gc.IsNil)
		c.Assert(w, gc.NotNil)
		n, err := w.Write([]byte(t.input))
		c.Assert(err, gc.IsNil)
		c.Assert(n, gc.Equals, len(t.input))
		c.Assert(buf.String(), gc.Equals, t.expected)
	}
}
