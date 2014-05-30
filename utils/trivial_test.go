// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package utils_test

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"path/filepath"
	"strings"

	"github.com/juju/testing"
	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/utils"
)

type utilsSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&utilsSuite{})

func (*utilsSuite) TestCompression(c *gc.C) {
	data := []byte(strings.Repeat("some data to be compressed\n", 100))
	compressedData := []byte{
		0x1f, 0x8b, 0x08, 0x00, 0x33, 0xb5, 0xf6, 0x50,
		0x00, 0x03, 0xed, 0xc9, 0xb1, 0x0d, 0x00, 0x20,
		0x08, 0x45, 0xc1, 0xde, 0x29, 0x58, 0x0d, 0xe5,
		0x97, 0x04, 0x23, 0xee, 0x1f, 0xa7, 0xb0, 0x7b,
		0xd7, 0x5e, 0x57, 0xca, 0xc2, 0xaf, 0xdb, 0x2d,
		0x9b, 0xb2, 0x55, 0xb9, 0x8f, 0xba, 0x15, 0xa3,
		0x29, 0x8a, 0xa2, 0x28, 0x8a, 0xa2, 0x28, 0xea,
		0x67, 0x3d, 0x71, 0x71, 0x6e, 0xbf, 0x8c, 0x0a,
		0x00, 0x00,
	}
	cdata := utils.Gzip(data)
	c.Assert(len(cdata) < len(data), gc.Equals, true)
	data1, err := utils.Gunzip(cdata)
	c.Assert(err, gc.IsNil)
	c.Assert(data1, gc.DeepEquals, data)

	data1, err = utils.Gunzip(compressedData)
	c.Assert(err, gc.IsNil)
	c.Assert(data1, gc.DeepEquals, data)
}

func (*utilsSuite) TestCommandString(c *gc.C) {
	type test struct {
		args     []string
		expected string
	}
	tests := []test{
		{nil, ""},
		{[]string{"a"}, "a"},
		{[]string{"a$"}, `"a\$"`},
		{[]string{""}, ""},
		{[]string{"\\"}, `"\\"`},
		{[]string{"a", "'b'"}, "a 'b'"},
		{[]string{"a b"}, `"a b"`},
		{[]string{"a", `"b"`}, `a "\"b\""`},
		{[]string{"a", `"b\"`}, `a "\"b\\\""`},
		{[]string{"a\n"}, "\"a\n\""},
	}
	for i, test := range tests {
		c.Logf("test %d: %q", i, test.args)
		result := utils.CommandString(test.args...)
		c.Assert(result, gc.Equals, test.expected)
	}
}

func (*utilsSuite) TestReadSHA256AndReadFileSHA256(c *gc.C) {
	sha256Tests := []struct {
		content string
		sha256  string
	}{{
		content: "",
		sha256:  "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
	}, {
		content: "some content",
		sha256:  "290f493c44f5d63d06b374d0a5abd292fae38b92cab2fae5efefe1b0e9347f56",
	}, {
		content: "foo",
		sha256:  "2c26b46b68ffc68ff99b453c1d30413413422d706483bfa0f98a5e886266e7ae",
	}, {
		content: "Foo",
		sha256:  "1cbec737f863e4922cee63cc2ebbfaafcd1cff8b790d8cfd2e6a5d550b648afa",
	}, {
		content: "multi\nline\ntext\nhere",
		sha256:  "c384f11c0294280792a44d9d6abb81f9fd991904cb7eb851a88311b04114231e",
	}}

	tempDir := c.MkDir()
	for i, test := range sha256Tests {
		c.Logf("test %d: %q -> %q", i, test.content, test.sha256)
		buf := bytes.NewBufferString(test.content)
		hash, size, err := utils.ReadSHA256(buf)
		c.Check(err, gc.IsNil)
		c.Check(hash, gc.Equals, test.sha256)
		c.Check(int(size), gc.Equals, len(test.content))

		tempFileName := filepath.Join(tempDir, fmt.Sprintf("sha256-%d", i))
		err = ioutil.WriteFile(tempFileName, []byte(test.content), 0644)
		c.Check(err, gc.IsNil)
		fileHash, fileSize, err := utils.ReadFileSHA256(tempFileName)
		c.Check(err, gc.IsNil)
		c.Check(fileHash, gc.Equals, hash)
		c.Check(fileSize, gc.Equals, size)
	}
}
