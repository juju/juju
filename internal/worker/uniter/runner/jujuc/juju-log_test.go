// Copyright 2012, 2013 Canonical Ltd.
// Copyright 2014 Cloudbase Solutions SRL
// Licensed under the AGPLv3, see LICENCE file for details.

package jujuc_test

import (
	"fmt"
	stdtesting "testing"

	"github.com/juju/errors"
	"github.com/juju/loggo/v2"
	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/internal/cmd"
	"github.com/juju/juju/internal/cmd/cmdtesting"
	internallogger "github.com/juju/juju/internal/logger"
	"github.com/juju/juju/internal/worker/uniter/runner/jujuc"
	"github.com/juju/juju/internal/worker/uniter/runner/jujuc/mocks"
)

type JujuLogSuite struct {
	relationSuite
}

func TestJujuLogSuite(t *stdtesting.T) { tc.Run(t, &JujuLogSuite{}) }
func (s *JujuLogSuite) newJujuLogCommand(c *tc.C) cmd.Command {
	ctx, _ := s.newHookContext(-1, "", "")
	cmd, err := jujuc.NewJujuLogCommand(ctx)
	c.Assert(err, tc.ErrorIsNil)
	return jujuc.NewJujucCommandWrappedForTest(cmd)
}

func (s *JujuLogSuite) newJujuLogCommandWithMocks(ctrl *gomock.Controller, name string) (cmd.Command, *mocks.MockJujuLogContext, *loggo.TestWriter) {
	ctx := mocks.NewMockJujuLogContext(ctrl)

	moduleName := fmt.Sprintf("unit.%s.juju-log", name)
	logCtx := loggo.NewContext(loggo.UNSPECIFIED)
	testWriter := &loggo.TestWriter{}
	logCtx.AddWriter("test", testWriter)

	ctx.EXPECT().GetLoggerByName(moduleName).Return(internallogger.WrapLoggo(logCtx.GetLogger(moduleName)))

	cmd := jujuc.NewJujuLogCommandWithMocks(ctx)
	return jujuc.NewJujucCommandWrappedForTest(cmd), ctx, testWriter
}

func (s *JujuLogSuite) TestRequiresMessage(c *tc.C) {
	cmd := s.newJujuLogCommand(c)
	err := cmdtesting.InitCommand(cmd, []string{})
	c.Assert(err, tc.ErrorMatches, "no message specified")
}

func (s *JujuLogSuite) TestLogInitMissingLevel(c *tc.C) {
	cmd := s.newJujuLogCommand(c)
	err := cmdtesting.InitCommand(cmd, []string{"-l"})
	c.Assert(err, tc.ErrorMatches, "option needs an argument.*")

	err = cmdtesting.InitCommand(cmd, []string{"--log-level"})
	c.Assert(err, tc.ErrorMatches, "option needs an argument.*")
}

func (s *JujuLogSuite) TestLogInitMissingMessage(c *tc.C) {
	cmd := s.newJujuLogCommand(c)
	err := cmdtesting.InitCommand(cmd, []string{"-l", "FATAL"})
	c.Assert(err, tc.ErrorMatches, "no message specified")

	err = cmdtesting.InitCommand(cmd, []string{"--log-level", "FATAL"})
	c.Assert(err, tc.ErrorMatches, "no message specified")
}

func (s *JujuLogSuite) TestLogDeprecation(c *tc.C) {
	cmd := s.newJujuLogCommand(c)
	ctx, err := cmdtesting.RunCommand(c, cmd, "--format", "foo", "msg")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(cmdtesting.Stderr(ctx), tc.Equals, "--format flag deprecated for command \"juju-log\"")
}

func (s *JujuLogSuite) TestRunWithNoErrorsLogsOnRun(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	messages := []string{"foo", "msg"}
	cmd, context, _ := s.newJujuLogCommandWithMocks(ctrl, "")

	relation := mocks.NewMockContextRelation(ctrl)
	relation.EXPECT().FakeId().Return("")

	context.EXPECT().HookRelation().Return(relation, nil)
	context.EXPECT().UnitName().Return("")

	ctx, err := cmdtesting.RunCommand(c, cmd, messages...)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(cmdtesting.Stderr(ctx), tc.Equals, "")
	c.Assert(cmdtesting.Stdout(ctx), tc.Equals, "")
}

func (s *JujuLogSuite) TestRunWithErrorIsNotImplementedLogsOnRun(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	messages := []string{"foo", "msg"}
	cmd, context, _ := s.newJujuLogCommandWithMocks(ctrl, "")

	context.EXPECT().HookRelation().Return(nil, errors.NotImplementedf("not implemented"))
	context.EXPECT().UnitName().Return("")

	ctx, err := cmdtesting.RunCommand(c, cmd, messages...)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(cmdtesting.Stderr(ctx), tc.Equals, "")
	c.Assert(cmdtesting.Stdout(ctx), tc.Equals, "")
}

func (s *JujuLogSuite) TestRunWithErrorIsNotFoundLogsOnRun(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	messages := []string{"foo", "msg"}
	cmd, context, _ := s.newJujuLogCommandWithMocks(ctrl, "")

	context.EXPECT().HookRelation().Return(nil, errors.NotFoundf("not found"))
	context.EXPECT().UnitName().Return("")

	ctx, err := cmdtesting.RunCommand(c, cmd, messages...)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(cmdtesting.Stderr(ctx), tc.Equals, "")
	c.Assert(cmdtesting.Stdout(ctx), tc.Equals, "")
}

func (s *JujuLogSuite) TestRunWithErrorDoesNotLogOnRun(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	messages := []string{"foo", "msg"}

	cmd, context, _ := s.newJujuLogCommandWithMocks(ctrl, "")

	context.EXPECT().HookRelation().Return(nil, errors.New("bad"))
	context.EXPECT().UnitName().Return("")

	ctx, err := cmdtesting.RunCommand(c, cmd, messages...)
	c.Assert(errors.Cause(err), tc.ErrorMatches, "bad")
	c.Assert(cmdtesting.Stderr(ctx), tc.Equals, "")
	c.Assert(cmdtesting.Stdout(ctx), tc.Equals, "")
}
