// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package jujuc_test

import (
	"github.com/golang/mock/gomock"
	"github.com/juju/cmd"
	"github.com/juju/cmd/cmdtesting"
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/worker/uniter/runner/jujuc"
	"github.com/juju/juju/worker/uniter/runner/jujuc/mocks"
)

type commitSuite struct {
	mockContext *mocks.MockContext
}

var _ = gc.Suite(&commitSuite{})

func (s *commitSuite) TestHelp(c *gc.C) {
	toolCmd, err := jujuc.NewCommand(nil, "commit")
	c.Assert(err, jc.ErrorIsNil)

	ctx := cmdtesting.Context(c)
	code := cmd.Main(jujuc.NewJujucCommandWrappedForTest(toolCmd), ctx, []string{"--help"})
	c.Check(code, gc.Equals, 0)
	c.Assert(bufferString(ctx.Stderr), gc.Equals, "")

	var expectedHelp = `
Usage: commit

Summary:
saves current charm data

Details:
commit saves current changes made by the charm.  This is automatically done if the
hook exists successfully.  If the hook does not exist successfully, changes are
rolled back and not saved.
`[1:]

	c.Assert(bufferString(ctx.Stdout), gc.Equals, expectedHelp)

}

func (s *commitSuite) TestRun(c *gc.C) {
	defer s.setupMocks(c).Finish()
	s.mockContext.EXPECT().Commit().Return(nil)
	s.runTest(c, 0, "")
}

func (s *commitSuite) TestRunFail(c *gc.C) {
	defer s.setupMocks(c).Finish()
	s.mockContext.EXPECT().Commit().Return(errors.New("fake error for test"))
	s.runTest(c, 1, "ERROR fake error for test\n")
}

func (s *commitSuite) runTest(c *gc.C, errCode int, errMsg string) {
	toolCmd, err := jujuc.NewCommand(s.mockContext, "commit")
	c.Assert(err, jc.ErrorIsNil)

	ctx := cmdtesting.Context(c)
	code := cmd.Main(jujuc.NewJujucCommandWrappedForTest(toolCmd), ctx, []string{})
	c.Check(code, gc.Equals, errCode)
	c.Assert(bufferString(ctx.Stderr), gc.Equals, errMsg)
	c.Assert(bufferString(ctx.Stdout), gc.Equals, "")
}

func (s *commitSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.mockContext = mocks.NewMockContext(ctrl)
	return ctrl
}
