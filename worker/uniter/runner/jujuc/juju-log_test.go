// Copyright 2012, 2013 Canonical Ltd.
// Copyright 2014 Cloudbase Solutions SRL
// Licensed under the AGPLv3, see LICENCE file for details.

package jujuc_test

import (
	"strings"

	"github.com/golang/mock/gomock"
	"github.com/juju/cmd"
	"github.com/juju/cmd/cmdtesting"
	"github.com/juju/errors"
	"github.com/juju/loggo"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/worker/uniter/runner/jujuc"
)

type JujuLogSuite struct {
	relationSuite
}

var _ = gc.Suite(&JujuLogSuite{})

func (s *JujuLogSuite) newJujuLogCommand(c *gc.C) cmd.Command {
	ctx, _ := s.newHookContext(-1, "")
	cmd, err := jujuc.NewJujuLogCommand(ctx)
	c.Assert(err, jc.ErrorIsNil)
	return cmd
}

func (s *JujuLogSuite) newJujuLogCommandWithMocks(c *gc.C, logger jujuc.JujuLogCommandLogger, ctx jujuc.JujuLogContext) cmd.Command {
	cmd, err := jujuc.NewJujuLogCommandWithMocks(ctx, logger)
	c.Assert(err, jc.ErrorIsNil)
	return cmd
}

func (s *JujuLogSuite) TestRequiresMessage(c *gc.C) {
	cmd := s.newJujuLogCommand(c)
	err := cmdtesting.InitCommand(cmd, []string{})
	c.Assert(err, gc.ErrorMatches, "no message specified")
}

func (s *JujuLogSuite) TestLogInitMissingLevel(c *gc.C) {
	cmd := s.newJujuLogCommand(c)
	err := cmdtesting.InitCommand(cmd, []string{"-l"})
	c.Assert(err, gc.ErrorMatches, "flag needs an argument.*")

	err = cmdtesting.InitCommand(cmd, []string{"--log-level"})
	c.Assert(err, gc.ErrorMatches, "flag needs an argument.*")
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

	logger := s.expectLogf(ctrl, loggo.INFO, ": ", messages...)

	relation := jujuc.NewMockContextRelation(ctrl)
	relation.EXPECT().FakeId().Return("")

	context := jujuc.NewMockJujuLogContext(ctrl)
	context.EXPECT().HookRelation().Return(relation, nil)
	context.EXPECT().UnitName().Return("")

	cmd := s.newJujuLogCommandWithMocks(c, logger, context)
	ctx, err := cmdtesting.RunCommand(c, cmd, messages...)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cmdtesting.Stderr(ctx), gc.Equals, "")
	c.Assert(cmdtesting.Stdout(ctx), gc.Equals, "")
}

func (s *JujuLogSuite) TestRunWithErrorIsNotImplementedLogsOnRun(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	messages := []string{"foo", "msg"}

	logger := s.expectLogf(ctrl, loggo.INFO, "", messages...)

	context := jujuc.NewMockJujuLogContext(ctrl)
	context.EXPECT().HookRelation().Return(nil, errors.NotImplementedf("not implemented"))
	context.EXPECT().UnitName().Return("")

	cmd := s.newJujuLogCommandWithMocks(c, logger, context)
	ctx, err := cmdtesting.RunCommand(c, cmd, messages...)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cmdtesting.Stderr(ctx), gc.Equals, "")
	c.Assert(cmdtesting.Stdout(ctx), gc.Equals, "")
}

func (s *JujuLogSuite) TestRunWithErrorIsNotFoundLogsOnRun(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	messages := []string{"foo", "msg"}

	logger := s.expectLogf(ctrl, loggo.INFO, "", messages...)

	context := jujuc.NewMockJujuLogContext(ctrl)
	context.EXPECT().HookRelation().Return(nil, errors.NotFoundf("not found"))
	context.EXPECT().UnitName().Return("")

	cmd := s.newJujuLogCommandWithMocks(c, logger, context)
	ctx, err := cmdtesting.RunCommand(c, cmd, messages...)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cmdtesting.Stderr(ctx), gc.Equals, "")
	c.Assert(cmdtesting.Stdout(ctx), gc.Equals, "")
}

func (s *JujuLogSuite) TestRunWithErrorDoesNotLogOnRun(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	messages := []string{"foo", "msg"}

	logger := jujuc.NewMockJujuLogCommandLogger(ctrl)

	context := jujuc.NewMockJujuLogContext(ctrl)
	context.EXPECT().HookRelation().Return(nil, errors.New("bad"))
	context.EXPECT().UnitName().Return("")

	cmd := s.newJujuLogCommandWithMocks(c, logger, context)
	ctx, err := cmdtesting.RunCommand(c, cmd, messages...)
	c.Assert(errors.Cause(err), gc.ErrorMatches, "bad")
	c.Assert(cmdtesting.Stderr(ctx), gc.Equals, "")
	c.Assert(cmdtesting.Stdout(ctx), gc.Equals, "")
}

func (s *JujuLogSuite) expectLogf(ctrl *gomock.Controller, level loggo.Level, prefix string, args ...string) jujuc.JujuLogCommandLogger {
	logger := jujuc.NewMockJujuLogCommandLogger(ctrl)
	logger.EXPECT().Logf(level, "%s%s", prefix, strings.Join(args, " "))
	return logger
}
