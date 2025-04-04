// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package jujuc_test

import (
	"github.com/juju/cmd/v3"
	"github.com/juju/cmd/v3/cmdtesting"
	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/internal/worker/uniter/runner/jujuc"
	"github.com/juju/juju/internal/worker/uniter/runner/jujuc/mocks"
)

var _ = gc.Suite(&ResourceGetCmdSuite{})

type ResourceGetCmdSuite struct {
	testing.IsolationSuite
}

func (s *ResourceGetCmdSuite) TestInitNilArgs(c *gc.C) {
	getCmd := jujuc.ResourceGetCmd{}
	err := getCmd.Init(nil)
	c.Check(err, gc.NotNil)
}

func (s *ResourceGetCmdSuite) TestInitTooFewArgs(c *gc.C) {
	getCmd := jujuc.ResourceGetCmd{}
	err := getCmd.Init([]string{})
	c.Check(err, gc.NotNil)
}

func (s *ResourceGetCmdSuite) TestInitTooManyArgs(c *gc.C) {
	getCmd := jujuc.ResourceGetCmd{}
	err := getCmd.Init([]string{"spam", "eggs"})
	c.Check(err, gc.NotNil)
}

func (s *ResourceGetCmdSuite) TestRun(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	const expected = "/var/lib/juju/agents/unit-foo-1/resources/spam/a-file.tgz"
	hctx := mocks.NewMockContext(ctrl)
	hctx.EXPECT().DownloadResource("spam").Return(expected, nil)

	com, err := jujuc.NewCommand(hctx, "resource-get")
	c.Assert(err, jc.ErrorIsNil)
	ctx := cmdtesting.Context(c)
	code := cmd.Main(jujuc.NewJujucCommandWrappedForTest(com), ctx, []string{"spam"})
	c.Assert(code, gc.Equals, 0)
	c.Check(cmdtesting.Stdout(ctx), gc.Equals, expected)
	c.Assert(cmdtesting.Stderr(ctx), gc.Equals, "")
}

func (s *ResourceGetCmdSuite) TestRunDownloadFailure(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	hctx := mocks.NewMockContext(ctrl)
	hctx.EXPECT().DownloadResource("spam").Return("", errors.New("<failure>"))

	com, err := jujuc.NewCommand(hctx, "resource-get")
	c.Assert(err, jc.ErrorIsNil)
	ctx := cmdtesting.Context(c)
	code := cmd.Main(jujuc.NewJujucCommandWrappedForTest(com), ctx, []string{"spam"})
	c.Assert(code, gc.Equals, 1)
	c.Check(cmdtesting.Stdout(ctx), gc.Equals, "")
	c.Assert(cmdtesting.Stderr(ctx), gc.Equals, "ERROR could not download resource: <failure>\n")
}
