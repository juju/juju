// Copyright 2017 Canonical Ltd.
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
	"github.com/juju/juju/worker/uniter/runner/jujuc"
)

type PayloadUnregisterSuite struct {
	ContextSuite

	hookCtx *stubUnregisterContext
	command cmd.Command
}

var _ = gc.Suite(&PayloadUnregisterSuite{})

func (s *PayloadUnregisterSuite) SetUpTest(c *gc.C) {
	s.ContextSuite.SetUpTest(c)

	s.hookCtx = &stubUnregisterContext{Stub: &testing.Stub{}}

	var err error
	s.command, err = jujuc.NewPayloadUnregisterCmd(s.hookCtx)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *PayloadUnregisterSuite) TestInitNilArgs(c *gc.C) {
	err := s.command.Init(nil)
	c.Assert(err, gc.NotNil)
}

func (s *PayloadUnregisterSuite) TestInitTooFewArgs(c *gc.C) {
	err := s.command.Init([]string{"foo"})
	c.Assert(err, gc.NotNil)
}

func (s *PayloadUnregisterSuite) TestRunOkay(c *gc.C) {
	err := s.command.Init([]string{"spam", "eggs"})
	c.Assert(err, jc.ErrorIsNil)

	err = s.runUnregisterCommand(c)
	c.Assert(err, jc.ErrorIsNil)

	s.hookCtx.CheckCalls(c, []testing.StubCall{{
		FuncName: "UntrackPayload",
		Args: []interface{}{params.UntrackPayloadParams{
			Class: "spam",
			ID:    "eggs",
		}},
	}})
}

func (s *PayloadUnregisterSuite) TestRunFail(c *gc.C) {
	s.hookCtx.SetErrors(errors.New("fail"))
	err := s.runUnregisterCommand(c)
	c.Assert(err, gc.ErrorMatches, "fail")
}

func (s *PayloadUnregisterSuite) runUnregisterCommand(c *gc.C) error {
	ctx := cmdtesting.Context(c)
	return s.command.Run(ctx)
}

type stubUnregisterContext struct {
	jujuc.Context
	*testing.Stub
}

func (s stubUnregisterContext) UntrackPayload(p params.UntrackPayloadParams) error {
	s.AddCall("UntrackPayload", p)
	return s.NextErr()
}
