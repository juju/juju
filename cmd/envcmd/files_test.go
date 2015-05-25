// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package envcmd_test

import (
	"io/ioutil"
	"os"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cmd/envcmd"
	"github.com/juju/juju/testing"
)

type filesSuite struct {
	testing.FakeJujuHomeSuite
}

var _ = gc.Suite(&filesSuite{})

func (s *filesSuite) TestReadCurrentEnvironmentUnset(c *gc.C) {
	env, err := envcmd.ReadCurrentEnvironment()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(env, gc.Equals, "")
}

func (s *filesSuite) TestReadCurrentSystemUnset(c *gc.C) {
	env, err := envcmd.ReadCurrentSystem()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(env, gc.Equals, "")
}

func (s *filesSuite) TestReadCurrentEnvironmentSet(c *gc.C) {
	err := envcmd.WriteCurrentEnvironment("fubar")
	c.Assert(err, jc.ErrorIsNil)
	env, err := envcmd.ReadCurrentEnvironment()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(env, gc.Equals, "fubar")
}

func (s *filesSuite) TestReadCurrentSystemSet(c *gc.C) {
	err := envcmd.WriteCurrentSystem("fubar")
	c.Assert(err, jc.ErrorIsNil)
	env, err := envcmd.ReadCurrentSystem()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(env, gc.Equals, "fubar")
}

func (s *filesSuite) TestWriteEnvironmentAddsNewline(c *gc.C) {
	err := envcmd.WriteCurrentEnvironment("fubar")
	c.Assert(err, jc.ErrorIsNil)
	current, err := ioutil.ReadFile(envcmd.GetCurrentEnvironmentFilePath())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(string(current), gc.Equals, "fubar\n")
}

func (s *filesSuite) TestWriteSystemAddsNewline(c *gc.C) {
	err := envcmd.WriteCurrentSystem("fubar")
	c.Assert(err, jc.ErrorIsNil)
	current, err := ioutil.ReadFile(envcmd.GetCurrentSystemFilePath())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(string(current), gc.Equals, "fubar\n")
}

func (s *filesSuite) TestWriteEnvironmentRemovesSystemFile(c *gc.C) {
	err := envcmd.WriteCurrentSystem("baz")
	c.Assert(err, jc.ErrorIsNil)
	err = envcmd.WriteCurrentEnvironment("fubar")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(envcmd.GetCurrentSystemFilePath(), jc.DoesNotExist)
}

func (s *filesSuite) TestWriteSystemRemovesEnvironmentFile(c *gc.C) {
	err := envcmd.WriteCurrentEnvironment("fubar")
	c.Assert(err, jc.ErrorIsNil)
	err = envcmd.WriteCurrentSystem("baz")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(envcmd.GetCurrentEnvironmentFilePath(), jc.DoesNotExist)
}

func (*filesSuite) TestErrorWritingCurrentEnvironment(c *gc.C) {
	// Can't write a file over a directory.
	os.MkdirAll(envcmd.GetCurrentEnvironmentFilePath(), 0777)
	err := envcmd.WriteCurrentEnvironment("fubar")
	c.Assert(err, gc.ErrorMatches, "unable to write to the environment file: .*")
}

func (*filesSuite) TestErrorWritingCurrentSystem(c *gc.C) {
	// Can't write a file over a directory.
	os.MkdirAll(envcmd.GetCurrentSystemFilePath(), 0777)
	err := envcmd.WriteCurrentSystem("fubar")
	c.Assert(err, gc.ErrorMatches, "unable to write to the system file: .*")
}
