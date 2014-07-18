// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing_test

import (
	"io"

	gc "launchpad.net/gocheck"

	"github.com/juju/juju/testing"
)

type TestingFilesSuite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&TestingFilesSuite{})

//---------------------------
// FakeFile.Read()

// Ensure the interface is satisfied.
var _ io.Reader = (*testing.FakeFile)(nil)

func (s *TestingFilesSuite) TestFakeFileReadError(c *gc.C) {
	file := testing.FakeFile{ReadError: "failed to read!"}
	buffer := make([]byte, 10)
	size, err := file.Read(buffer)

	c.Check(size, gc.Equals, 0)
	c.Check(err, gc.ErrorMatches, "failed to read!")
}

//---------------------------
// FakeFile.Read()

// Ensure the interface is satisfied.
var _ io.Writer = (*testing.FakeFile)(nil)

func (s *TestingFilesSuite) TestFakeFileWriteError(c *gc.C) {
	file := testing.FakeFile{WriteError: "failed to write!"}
	buffer := []byte("data")
	size, err := file.Write(buffer)

	c.Check(size, gc.Equals, 0)
	c.Check(err, gc.ErrorMatches, "failed to write!")
}

//---------------------------
// FakeFile.Close()

// Ensure the interface is satisfied.
var _ io.Closer = (*testing.FakeFile)(nil)

func (s *TestingFilesSuite) TestFakeFileCloseError(c *gc.C) {
	file := testing.FakeFile{CloseError: "failed to close!"}
	err := file.Close()

	c.Check(err, gc.ErrorMatches, "failed to close!")
}
