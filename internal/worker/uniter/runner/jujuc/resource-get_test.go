// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package jujuc_test

import (
	"github.com/juju/errors"
	"github.com/juju/tc"
	"github.com/juju/testing"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/internal/cmd"
	"github.com/juju/juju/internal/cmd/cmdtesting"
	"github.com/juju/juju/internal/worker/uniter/runner/jujuc"
	"github.com/juju/juju/internal/worker/uniter/runner/jujuc/mocks"
)

var _ = tc.Suite(&ResourceGetCmdSuite{})

type ResourceGetCmdSuite struct {
	testing.IsolationSuite
}

func (s *ResourceGetCmdSuite) TestInitNilArgs(c *tc.C) {
	getCmd := jujuc.ResourceGetCmd{}
	err := getCmd.Init(nil)
	c.Check(err, tc.NotNil)
}

func (s *ResourceGetCmdSuite) TestInitTooFewArgs(c *tc.C) {
	getCmd := jujuc.ResourceGetCmd{}
	err := getCmd.Init([]string{})
	c.Check(err, tc.NotNil)
}

func (s *ResourceGetCmdSuite) TestInitTooManyArgs(c *tc.C) {
	getCmd := jujuc.ResourceGetCmd{}
	err := getCmd.Init([]string{"spam", "eggs"})
	c.Check(err, tc.NotNil)
}

func (s *ResourceGetCmdSuite) TestRun(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	const expected = "/var/lib/juju/agents/unit-foo-1/resources/spam/a-file.tgz"
	hctx := mocks.NewMockContext(ctrl)
	hctx.EXPECT().DownloadResource(gomock.Any(), "spam").Return(expected, nil)

	com, err := jujuc.NewCommand(hctx, "resource-get")
	c.Assert(err, tc.ErrorIsNil)
	ctx := cmdtesting.Context(c)
	code := cmd.Main(jujuc.NewJujucCommandWrappedForTest(com), ctx, []string{"spam"})
	c.Assert(code, tc.Equals, 0)
	c.Check(cmdtesting.Stdout(ctx), tc.Equals, expected)
	c.Assert(cmdtesting.Stderr(ctx), tc.Equals, "")
}

func (s *ResourceGetCmdSuite) TestRunDownloadFailure(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	hctx := mocks.NewMockContext(ctrl)
	hctx.EXPECT().DownloadResource(gomock.Any(), "spam").Return("", errors.New("<failure>"))

	com, err := jujuc.NewCommand(hctx, "resource-get")
	c.Assert(err, tc.ErrorIsNil)
	ctx := cmdtesting.Context(c)
	code := cmd.Main(jujuc.NewJujucCommandWrappedForTest(com), ctx, []string{"spam"})
	c.Assert(code, tc.Equals, 1)
	c.Check(cmdtesting.Stdout(ctx), tc.Equals, "")
	c.Assert(cmdtesting.Stderr(ctx), tc.Equals, "ERROR could not download resource: <failure>\n")
}
