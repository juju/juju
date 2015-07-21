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

func (s *filesSuite) assertCurrentEnvironment(c *gc.C, environmentName string) {
	current, err := envcmd.ReadCurrentEnvironment()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(current, gc.Equals, environmentName)
}

func (s *filesSuite) assertCurrentSystem(c *gc.C, systemName string) {
	current, err := envcmd.ReadCurrentSystem()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(current, gc.Equals, systemName)
}

func (s *filesSuite) TestReadCurrentEnvironmentUnset(c *gc.C) {
	s.assertCurrentEnvironment(c, "")
}

func (s *filesSuite) TestReadCurrentSystemUnset(c *gc.C) {
	s.assertCurrentSystem(c, "")
}

func (s *filesSuite) TestReadCurrentEnvironmentSet(c *gc.C) {
	err := envcmd.WriteCurrentEnvironment("fubar")
	c.Assert(err, jc.ErrorIsNil)
	s.assertCurrentEnvironment(c, "fubar")
}

func (s *filesSuite) TestReadCurrentSystemSet(c *gc.C) {
	err := envcmd.WriteCurrentSystem("fubar")
	c.Assert(err, jc.ErrorIsNil)
	s.assertCurrentSystem(c, "fubar")
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

func (*filesSuite) TestCurrentCommenctionNameMissing(c *gc.C) {
	name, isSystem, err := envcmd.CurrentConnectionName()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(isSystem, jc.IsFalse)
	c.Assert(name, gc.Equals, "")
}

func (*filesSuite) TestCurrentCommenctionNameEnvironment(c *gc.C) {
	err := envcmd.WriteCurrentEnvironment("fubar")
	c.Assert(err, jc.ErrorIsNil)
	name, isSystem, err := envcmd.CurrentConnectionName()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(isSystem, jc.IsFalse)
	c.Assert(name, gc.Equals, "fubar")
}

func (*filesSuite) TestCurrentCommenctionNameSystem(c *gc.C) {
	err := envcmd.WriteCurrentSystem("baz")
	c.Assert(err, jc.ErrorIsNil)
	name, isSystem, err := envcmd.CurrentConnectionName()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(isSystem, jc.IsTrue)
	c.Assert(name, gc.Equals, "baz")
}

func (s *filesSuite) TestSetCurrentEnvironment(c *gc.C) {
	ctx := testing.Context(c)
	err := envcmd.SetCurrentEnvironment(ctx, "new-env")
	c.Assert(err, jc.ErrorIsNil)
	s.assertCurrentEnvironment(c, "new-env")
	c.Assert(testing.Stderr(ctx), gc.Equals, "-> new-env\n")
}

func (s *filesSuite) TestSetCurrentEnvironmentExistingEnv(c *gc.C) {
	err := envcmd.WriteCurrentEnvironment("fubar")
	c.Assert(err, jc.ErrorIsNil)
	ctx := testing.Context(c)
	err = envcmd.SetCurrentEnvironment(ctx, "new-env")
	c.Assert(err, jc.ErrorIsNil)
	s.assertCurrentEnvironment(c, "new-env")
	c.Assert(testing.Stderr(ctx), gc.Equals, "fubar -> new-env\n")
}

func (s *filesSuite) TestSetCurrentEnvironmentExistingSystem(c *gc.C) {
	err := envcmd.WriteCurrentSystem("fubar")
	c.Assert(err, jc.ErrorIsNil)
	ctx := testing.Context(c)
	err = envcmd.SetCurrentEnvironment(ctx, "new-env")
	c.Assert(err, jc.ErrorIsNil)
	s.assertCurrentEnvironment(c, "new-env")
	c.Assert(testing.Stderr(ctx), gc.Equals, "fubar (system) -> new-env\n")
}

func (s *filesSuite) TestSetCurrentSystem(c *gc.C) {
	ctx := testing.Context(c)
	err := envcmd.SetCurrentSystem(ctx, "new-sys")
	c.Assert(err, jc.ErrorIsNil)
	s.assertCurrentSystem(c, "new-sys")
	c.Assert(testing.Stderr(ctx), gc.Equals, "-> new-sys (system)\n")
}

func (s *filesSuite) TestSetCurrentSystemExistingEnv(c *gc.C) {
	err := envcmd.WriteCurrentEnvironment("fubar")
	c.Assert(err, jc.ErrorIsNil)
	ctx := testing.Context(c)
	err = envcmd.SetCurrentSystem(ctx, "new-sys")
	c.Assert(err, jc.ErrorIsNil)
	s.assertCurrentSystem(c, "new-sys")
	c.Assert(testing.Stderr(ctx), gc.Equals, "fubar -> new-sys (system)\n")
}

func (s *filesSuite) TestSetCurrentSystemExistingSystem(c *gc.C) {
	err := envcmd.WriteCurrentSystem("fubar")
	c.Assert(err, jc.ErrorIsNil)
	ctx := testing.Context(c)
	err = envcmd.SetCurrentSystem(ctx, "new-sys")
	c.Assert(err, jc.ErrorIsNil)
	s.assertCurrentSystem(c, "new-sys")
	c.Assert(testing.Stderr(ctx), gc.Equals, "fubar (system) -> new-sys (system)\n")
}
