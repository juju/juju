// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelcmd_test

import (
	"io/ioutil"
	"os"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/testing"
)

type filesSuite struct {
	testing.FakeJujuXDGDataHomeSuite
}

var _ = gc.Suite(&filesSuite{})

func (s *filesSuite) assertCurrentController(c *gc.C, controllerName string) {
	current, err := modelcmd.ReadCurrentController()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(current, gc.Equals, controllerName)
}

func (s *filesSuite) TestReadCurrentControllerUnset(c *gc.C) {
	s.assertCurrentController(c, "")
}

func (s *filesSuite) TestReadCurrentControllerSet(c *gc.C) {
	err := modelcmd.WriteCurrentController("fubar")
	c.Assert(err, jc.ErrorIsNil)
	s.assertCurrentController(c, "fubar")
}

func (s *filesSuite) TestWriteControllerAddsNewline(c *gc.C) {
	err := modelcmd.WriteCurrentController("fubar")
	c.Assert(err, jc.ErrorIsNil)
	current, err := ioutil.ReadFile(modelcmd.GetCurrentControllerFilePath())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(string(current), gc.Equals, "fubar\n")
}

func (*filesSuite) TestErrorWritingCurrentController(c *gc.C) {
	// Can't write a file over a directory.
	os.MkdirAll(modelcmd.GetCurrentControllerFilePath(), 0777)
	err := modelcmd.WriteCurrentController("fubar")
	c.Assert(err, gc.ErrorMatches, "unable to write to the controller file: .*")
}

func (s *filesSuite) TestWriteCurrentControllerExistingController(c *gc.C) {
	err := modelcmd.WriteCurrentController("fubar")
	c.Assert(err, jc.ErrorIsNil)
	err = modelcmd.WriteCurrentController("new-sys")
	c.Assert(err, jc.ErrorIsNil)
	s.assertCurrentController(c, "new-sys")
}
