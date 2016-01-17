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

func (s *filesSuite) assertCurrentController(c *gc.C, controllerName string) {
	current, err := envcmd.ReadCurrentController()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(current, gc.Equals, controllerName)
}

func (s *filesSuite) TestReadCurrentEnvironmentUnset(c *gc.C) {
	s.assertCurrentEnvironment(c, "")
}

func (s *filesSuite) TestReadCurrentControllerUnset(c *gc.C) {
	s.assertCurrentController(c, "")
}

func (s *filesSuite) TestReadCurrentEnvironmentSet(c *gc.C) {
	err := envcmd.WriteCurrentEnvironment("fubar")
	c.Assert(err, jc.ErrorIsNil)
	s.assertCurrentEnvironment(c, "fubar")
}

func (s *filesSuite) TestReadCurrentControllerSet(c *gc.C) {
	err := envcmd.WriteCurrentController("fubar")
	c.Assert(err, jc.ErrorIsNil)
	s.assertCurrentController(c, "fubar")
}

func (s *filesSuite) TestWriteEnvironmentAddsNewline(c *gc.C) {
	err := envcmd.WriteCurrentEnvironment("fubar")
	c.Assert(err, jc.ErrorIsNil)
	current, err := ioutil.ReadFile(envcmd.GetCurrentEnvironmentFilePath())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(string(current), gc.Equals, "fubar\n")
}

func (s *filesSuite) TestWriteControllerAddsNewline(c *gc.C) {
	err := envcmd.WriteCurrentController("fubar")
	c.Assert(err, jc.ErrorIsNil)
	current, err := ioutil.ReadFile(envcmd.GetCurrentControllerFilePath())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(string(current), gc.Equals, "fubar\n")
}

func (s *filesSuite) TestWriteEnvironmentRemovesControllerFile(c *gc.C) {
	err := envcmd.WriteCurrentController("baz")
	c.Assert(err, jc.ErrorIsNil)
	err = envcmd.WriteCurrentEnvironment("fubar")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(envcmd.GetCurrentControllerFilePath(), jc.DoesNotExist)
}

func (s *filesSuite) TestWriteControllerRemovesEnvironmentFile(c *gc.C) {
	err := envcmd.WriteCurrentEnvironment("fubar")
	c.Assert(err, jc.ErrorIsNil)
	err = envcmd.WriteCurrentController("baz")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(envcmd.GetCurrentEnvironmentFilePath(), jc.DoesNotExist)
}

func (*filesSuite) TestErrorWritingCurrentEnvironment(c *gc.C) {
	// Can't write a file over a directory.
	os.MkdirAll(envcmd.GetCurrentEnvironmentFilePath(), 0777)
	err := envcmd.WriteCurrentEnvironment("fubar")
	c.Assert(err, gc.ErrorMatches, "unable to write to the environment file: .*")
}

func (*filesSuite) TestErrorWritingCurrentController(c *gc.C) {
	// Can't write a file over a directory.
	os.MkdirAll(envcmd.GetCurrentControllerFilePath(), 0777)
	err := envcmd.WriteCurrentController("fubar")
	c.Assert(err, gc.ErrorMatches, "unable to write to the controller file: .*")
}

func (*filesSuite) TestCurrentCommenctionNameMissing(c *gc.C) {
	name, isController, err := envcmd.CurrentConnectionName()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(isController, jc.IsFalse)
	c.Assert(name, gc.Equals, "")
}

func (*filesSuite) TestCurrentCommenctionNameEnvironment(c *gc.C) {
	err := envcmd.WriteCurrentEnvironment("fubar")
	c.Assert(err, jc.ErrorIsNil)
	name, isController, err := envcmd.CurrentConnectionName()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(isController, jc.IsFalse)
	c.Assert(name, gc.Equals, "fubar")
}

func (*filesSuite) TestCurrentCommenctionNameController(c *gc.C) {
	err := envcmd.WriteCurrentController("baz")
	c.Assert(err, jc.ErrorIsNil)
	name, isController, err := envcmd.CurrentConnectionName()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(isController, jc.IsTrue)
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

func (s *filesSuite) TestSetCurrentEnvironmentExistingController(c *gc.C) {
	err := envcmd.WriteCurrentController("fubar")
	c.Assert(err, jc.ErrorIsNil)
	ctx := testing.Context(c)
	err = envcmd.SetCurrentEnvironment(ctx, "new-env")
	c.Assert(err, jc.ErrorIsNil)
	s.assertCurrentEnvironment(c, "new-env")
	c.Assert(testing.Stderr(ctx), gc.Equals, "fubar (controller) -> new-env\n")
}

func (s *filesSuite) TestSetCurrentController(c *gc.C) {
	ctx := testing.Context(c)
	err := envcmd.SetCurrentController(ctx, "new-sys")
	c.Assert(err, jc.ErrorIsNil)
	s.assertCurrentController(c, "new-sys")
	c.Assert(testing.Stderr(ctx), gc.Equals, "-> new-sys (controller)\n")
}

func (s *filesSuite) TestSetCurrentControllerExistingEnv(c *gc.C) {
	err := envcmd.WriteCurrentEnvironment("fubar")
	c.Assert(err, jc.ErrorIsNil)
	ctx := testing.Context(c)
	err = envcmd.SetCurrentController(ctx, "new-sys")
	c.Assert(err, jc.ErrorIsNil)
	s.assertCurrentController(c, "new-sys")
	c.Assert(testing.Stderr(ctx), gc.Equals, "fubar -> new-sys (controller)\n")
}

func (s *filesSuite) TestSetCurrentControllerExistingController(c *gc.C) {
	err := envcmd.WriteCurrentController("fubar")
	c.Assert(err, jc.ErrorIsNil)
	ctx := testing.Context(c)
	err = envcmd.SetCurrentController(ctx, "new-sys")
	c.Assert(err, jc.ErrorIsNil)
	s.assertCurrentController(c, "new-sys")
	c.Assert(testing.Stderr(ctx), gc.Equals, "fubar (controller) -> new-sys (controller)\n")
}
