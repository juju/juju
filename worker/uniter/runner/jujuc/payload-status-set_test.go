// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package jujuc_test

import (
	"github.com/juju/cmd"
	"github.com/juju/cmd/cmdtesting"
	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/payload"
	"github.com/juju/juju/worker/uniter/runner/jujuc"
)

type PayloadStatusSetSuite struct {
	ContextSuite

	hookCtx *stubPayloadSetStatusContext
	command cmd.Command
}

var _ = gc.Suite(&PayloadStatusSetSuite{})

func (s *PayloadStatusSetSuite) SetUpTest(c *gc.C) {
	s.ContextSuite.SetUpTest(c)
	s.hookCtx = &stubPayloadSetStatusContext{Stub: &testing.Stub{}}
	var err error
	s.command, err = jujuc.NewPayloadStatusSetCmd(s.hookCtx)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *PayloadStatusSetSuite) init(c *gc.C, class, id, status string) {
	err := s.command.Init([]string{class, id, status})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *PayloadStatusSetSuite) TestTooFewArgs(c *gc.C) {
	err := s.command.Init([]string{})
	c.Check(err, gc.ErrorMatches, `missing .*`)

	err = s.command.Init([]string{payload.StateRunning})
	c.Check(err, gc.ErrorMatches, `missing .*`)
}

func (s *PayloadStatusSetSuite) runPayloadStatusCommand(c *gc.C) error {
	ctx := cmdtesting.Context(c)
	return s.command.Run(ctx)
}

func (s *PayloadStatusSetSuite) TestStatusSet(c *gc.C) {
	s.init(c, "docker", "foo", payload.StateStopped)
	err := s.runPayloadStatusCommand(c)
	c.Check(err, jc.ErrorIsNil)
}

func (s *PayloadStatusSetSuite) TestStatusFail(c *gc.C) {
	s.hookCtx.SetErrors(errors.New("fail"))
	err := s.runPayloadStatusCommand(c)
	c.Assert(err, gc.ErrorMatches, "fail")
}

type stubPayloadSetStatusContext struct {
	jujuc.Context
	*testing.Stub
}

func (s stubPayloadSetStatusContext) SetPayloadStatus(p params.PayloadStatusParams) error {
	s.AddCall("SetPayloadStatus", p)
	return s.NextErr()
}
