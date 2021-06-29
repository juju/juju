// Copyright 2012, 2013 Canonical Ltd.
// Copyright 2014 Cloudbase Solutions SRL
// Licensed under the AGPLv3, see LICENCE file for details.

package jujuc_test

import (
	"fmt"
	"strings"

	"github.com/golang/mock/gomock"
	"github.com/juju/cmd"
	"github.com/juju/cmd/cmdtesting"
	"github.com/juju/errors"
	"github.com/juju/loggo/v2"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/worker/uniter/runner/jujuc"
	"github.com/juju/juju/worker/uniter/runner/jujuc/mocks"
)

type JujuLogSuite struct {
	relationSuite
}

var _ = gc.Suite(&JujuLogSuite{})

func (s *JujuLogSuite) newJujuLogCommand(c *gc.C) cmd.Command {
	ctx, _ := s.newHookContext(-1, "", "")
	cmd, err := jujuc.NewJujuLogCommand(ctx)
	c.Assert(err, jc.ErrorIsNil)
	return jujuc.NewJujucCommandWrappedForTest(cmd)
}

func (s *JujuLogSuite) newJujuLogCommandWithMocks(ctrl *gomock.Controller, name string) (cmd.Command, *mocks.MockJujuLogContext, *mocks.MockJujuLogCommandLogger) {
	logger := mocks.NewMockJujuLogCommandLogger(ctrl)

	factory := mocks.NewMockJujuLogCommandLoggerFactory(ctrl)
	factory.EXPECT().GetLogger(fmt.Sprintf("unit.%s.juju-log", name)).Return(logger)

	ctx := mocks.NewMockJujuLogContext(ctrl)

	cmd := jujuc.NewJujuLogCommandWithMocks(ctx, factory)
	return jujuc.NewJujucCommandWrappedForTest(cmd), ctx, logger
}

func (s *JujuLogSuite) TestRequiresMessage(c *gc.C) {
	cmd := s.newJujuLogCommand(c)
	err := cmdtesting.InitCommand(cmd, []string{})
	c.Assert(err, gc.ErrorMatches, "no message specified")
}

func (s *JujuLogSuite) TestLogInitMissingLevel(c *gc.C) {
	cmd := s.newJujuLogCommand(c)
	err := cmdtesting.InitCommand(cmd, []string{"-l"})
	c.Assert(err, gc.ErrorMatches, "option needs an argument.*")

	err = cmdtesting.InitCommand(cmd, []string{"--log-level"})
	c.Assert(err, gc.ErrorMatches, "option needs an argument.*")
}

func (s *JujuLogSuite) TestLogInitMissingMessage(c *gc.C) {
	cmd := s.newJujuLogCommand(c)
	err := cmdtesting.InitCommand(cmd, []string{"-l", "FATAL"})
	c.Assert(err, gc.ErrorMatches, "no message specified")

	err = cmdtesting.InitCommand(cmd, []string{"--log-level", "FATAL"})
	c.Assert(err, gc.ErrorMatches, "no message specified")
}

func (s *JujuLogSuite) TestLogDeprecation(c *gc.C) {
	cmd := s.newJujuLogCommand(c)
	ctx, err := cmdtesting.RunCommand(c, cmd, "--format", "foo", "msg")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cmdtesting.Stderr(ctx), gc.Equals, "--format flag deprecated for command \"juju-log\"")
}

func (s *JujuLogSuite) TestRunWithNoErrorsLogsOnRun(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	messages := []string{"foo", "msg"}

	cmd, context, logger := s.newJujuLogCommandWithMocks(ctrl, "")
	logger.EXPECT().Logf(loggo.INFO, "%s%s", ": ", strings.Join(messages, " "))

	relation := mocks.NewMockContextRelation(ctrl)
	relation.EXPECT().FakeId().Return("")

	context.EXPECT().HookRelation().Return(relation, nil)
	context.EXPECT().UnitName().Return("")

	ctx, err := cmdtesting.RunCommand(c, cmd, messages...)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cmdtesting.Stderr(ctx), gc.Equals, "")
	c.Assert(cmdtesting.Stdout(ctx), gc.Equals, "")
}

func (s *JujuLogSuite) TestRunWithErrorIsNotImplementedLogsOnRun(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	messages := []string{"foo", "msg"}

	cmd, context, logger := s.newJujuLogCommandWithMocks(ctrl, "")
	logger.EXPECT().Logf(loggo.INFO, "%s%s", "", strings.Join(messages, " "))

	context.EXPECT().HookRelation().Return(nil, errors.NotImplementedf("not implemented"))
	context.EXPECT().UnitName().Return("")

	ctx, err := cmdtesting.RunCommand(c, cmd, messages...)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cmdtesting.Stderr(ctx), gc.Equals, "")
	c.Assert(cmdtesting.Stdout(ctx), gc.Equals, "")
}

func (s *JujuLogSuite) TestRunWithErrorIsNotFoundLogsOnRun(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	messages := []string{"foo", "msg"}

	cmd, context, logger := s.newJujuLogCommandWithMocks(ctrl, "")
	logger.EXPECT().Logf(loggo.INFO, "%s%s", "", strings.Join(messages, " "))

	context.EXPECT().HookRelation().Return(nil, errors.NotFoundf("not found"))
	context.EXPECT().UnitName().Return("")

	ctx, err := cmdtesting.RunCommand(c, cmd, messages...)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cmdtesting.Stderr(ctx), gc.Equals, "")
	c.Assert(cmdtesting.Stdout(ctx), gc.Equals, "")
}

func (s *JujuLogSuite) TestRunWithErrorDoesNotLogOnRun(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	messages := []string{"foo", "msg"}

	cmd, context, _ := s.newJujuLogCommandWithMocks(ctrl, "")

	context.EXPECT().HookRelation().Return(nil, errors.New("bad"))
	context.EXPECT().UnitName().Return("")

	ctx, err := cmdtesting.RunCommand(c, cmd, messages...)
	c.Assert(errors.Cause(err), gc.ErrorMatches, "bad")
	c.Assert(cmdtesting.Stderr(ctx), gc.Equals, "")
	c.Assert(cmdtesting.Stdout(ctx), gc.Equals, "")
}
