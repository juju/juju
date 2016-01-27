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

func (s *filesSuite) assertCurrentModel(c *gc.C, environmentName string) {
	current, err := envcmd.ReadCurrentModel()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(current, gc.Equals, environmentName)
}

func (s *filesSuite) assertCurrentController(c *gc.C, controllerName string) {
	current, err := envcmd.ReadCurrentController()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(current, gc.Equals, controllerName)
}

func (s *filesSuite) TestReadCurrentModelUnset(c *gc.C) {
	s.assertCurrentModel(c, "")
}

func (s *filesSuite) TestReadCurrentControllerUnset(c *gc.C) {
	s.assertCurrentController(c, "")
}

func (s *filesSuite) TestReadCurrentModelSet(c *gc.C) {
	err := envcmd.WriteCurrentModel("fubar")
	c.Assert(err, jc.ErrorIsNil)
	s.assertCurrentModel(c, "fubar")
}

func (s *filesSuite) TestReadCurrentControllerSet(c *gc.C) {
	err := envcmd.WriteCurrentController("fubar")
	c.Assert(err, jc.ErrorIsNil)
	s.assertCurrentController(c, "fubar")
}

func (s *filesSuite) TestWriteEnvironmentAddsNewline(c *gc.C) {
	err := envcmd.WriteCurrentModel("fubar")
	c.Assert(err, jc.ErrorIsNil)
	current, err := ioutil.ReadFile(envcmd.GetCurrentModelFilePath())
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
	err = envcmd.WriteCurrentModel("fubar")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(envcmd.GetCurrentControllerFilePath(), jc.DoesNotExist)
}

func (s *filesSuite) TestWriteControllerRemovesEnvironmentFile(c *gc.C) {
	err := envcmd.WriteCurrentModel("fubar")
	c.Assert(err, jc.ErrorIsNil)
	err = envcmd.WriteCurrentController("baz")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(envcmd.GetCurrentModelFilePath(), jc.DoesNotExist)
}

func (*filesSuite) TestErrorWritingCurrentModel(c *gc.C) {
	// Can't write a file over a directory.
	os.MkdirAll(envcmd.GetCurrentModelFilePath(), 0777)
	err := envcmd.WriteCurrentModel("fubar")
	c.Assert(err, gc.ErrorMatches, "unable to write to the model file: .*")
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
	err := envcmd.WriteCurrentModel("fubar")
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

func (s *filesSuite) TestSetCurrentModel(c *gc.C) {
	ctx := testing.Context(c)
	err := envcmd.SetCurrentModel(ctx, "new-model")
	c.Assert(err, jc.ErrorIsNil)
	s.assertCurrentModel(c, "new-model")
	c.Assert(testing.Stderr(ctx), gc.Equals, "-> new-model\n")
}

func (s *filesSuite) TestSetCurrentModelExistingEnv(c *gc.C) {
	err := envcmd.WriteCurrentModel("fubar")
	c.Assert(err, jc.ErrorIsNil)
	ctx := testing.Context(c)
	err = envcmd.SetCurrentModel(ctx, "new-model")
	c.Assert(err, jc.ErrorIsNil)
	s.assertCurrentModel(c, "new-model")
	c.Assert(testing.Stderr(ctx), gc.Equals, "fubar -> new-model\n")
}

func (s *filesSuite) TestSetCurrentModelExistingController(c *gc.C) {
	err := envcmd.WriteCurrentController("fubar")
	c.Assert(err, jc.ErrorIsNil)
	ctx := testing.Context(c)
	err = envcmd.SetCurrentModel(ctx, "new-model")
	c.Assert(err, jc.ErrorIsNil)
	s.assertCurrentModel(c, "new-model")
	c.Assert(testing.Stderr(ctx), gc.Equals, "fubar (controller) -> new-model\n")
}

func (s *filesSuite) TestSetCurrentController(c *gc.C) {
	ctx := testing.Context(c)
	err := envcmd.SetCurrentController(ctx, "new-sys")
	c.Assert(err, jc.ErrorIsNil)
	s.assertCurrentController(c, "new-sys")
	c.Assert(testing.Stderr(ctx), gc.Equals, "-> new-sys (controller)\n")
}

func (s *filesSuite) TestSetCurrentControllerExistingEnv(c *gc.C) {
	err := envcmd.WriteCurrentModel("fubar")
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
