// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cmd

import (
	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	coretesting "github.com/juju/juju/testing"
)

var _ = gc.Suite(&GetCmdSuite{})

type GetCmdSuite struct {
	testing.IsolationSuite

	stub *testing.Stub
	hctx *stubHookContext
}

func (s *GetCmdSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)

	s.stub = &testing.Stub{}
	s.hctx = &stubHookContext{stub: s.stub}
}

func (s *GetCmdSuite) TestInitNilArgs(c *gc.C) {
	getCmd := GetCmd{}

	err := getCmd.Init(nil)

	c.Check(err, gc.NotNil)
}

func (s *GetCmdSuite) TestInitTooFewArgs(c *gc.C) {
	getCmd := GetCmd{}

	err := getCmd.Init([]string{})

	c.Check(err, gc.NotNil)
}

func (s *GetCmdSuite) TestInitTooManyArgs(c *gc.C) {
	getCmd := GetCmd{}

	err := getCmd.Init([]string{"spam", "eggs"})

	c.Check(err, gc.NotNil)
}

func (s *GetCmdSuite) TestInit(c *gc.C) {
	getCmd := GetCmd{}

	err := getCmd.Init([]string{"spam"})
	c.Assert(err, jc.ErrorIsNil)

	c.Check(getCmd.resourceName, gc.Equals, "spam")
}

func (s *GetCmdSuite) TestRunOkay(c *gc.C) {
	getCmd := GetCmd{
		hookContext:  s.hctx,
		resourceName: "spam",
	}
	const expected = "/var/lib/juju/agents/unit-foo-1/resources/spam/a-file.tgz"
	s.hctx.ReturnDownload = expected
	ctx := coretesting.Context(c)

	err := getCmd.Run(ctx)
	c.Assert(err, jc.ErrorIsNil)

	s.stub.CheckCallNames(c, "Download")
	s.stub.CheckCall(c, 0, "Download", "spam")
	c.Check(coretesting.Stdout(ctx), gc.Equals, expected)
	c.Check(coretesting.Stderr(ctx), gc.Equals, "")
}

func (s *GetCmdSuite) TestRunDownloadFailure(c *gc.C) {
	getCmd := GetCmd{
		hookContext:  s.hctx,
		resourceName: "spam",
	}
	failure := errors.New("<failure>")
	s.stub.SetErrors(failure)
	ctx := coretesting.Context(c)

	err := getCmd.Run(ctx)

	s.stub.CheckCallNames(c, "Download")
	c.Check(errors.Cause(err), gc.Equals, failure)
	c.Check(coretesting.Stdout(ctx), gc.Equals, "")
	c.Check(coretesting.Stderr(ctx), gc.Equals, "")
}
