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

type PayloadRegisterSuite struct {
	ContextSuite

	hookCtx *stubPayloadRegisterContext
	command cmd.Command
}

var _ = gc.Suite(&PayloadRegisterSuite{})

func (s *PayloadRegisterSuite) SetUpTest(c *gc.C) {
	s.ContextSuite.SetUpTest(c)

	s.hookCtx = &stubPayloadRegisterContext{Stub: &testing.Stub{}}

	var err error
	s.command, err = jujuc.NewPayloadRegisterCmd(s.hookCtx)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *PayloadRegisterSuite) TestInitNilArgs(c *gc.C) {
	err := s.command.Init(nil)
	c.Assert(err, gc.NotNil)
}

func (s *PayloadRegisterSuite) TestInitTooFewArgs(c *gc.C) {
	err := s.command.Init([]string{"foo", "bar"})
	c.Assert(err, gc.NotNil)
}

func (s *PayloadRegisterSuite) TestRun(c *gc.C) {
	err := s.command.Init([]string{"type", "class", "id", "tag1", "tag 2"})
	c.Assert(err, jc.ErrorIsNil)

	err = s.runRegisterCommand(c)
	c.Assert(err, jc.ErrorIsNil)
	s.hookCtx.CheckCalls(c, []testing.StubCall{{
		FuncName: "TrackPayload",
		Args: []interface{}{
			params.TrackPayloadParams{
				Class:  "class",
				Type:   "type",
				ID:     "id",
				Status: payload.StateRunning,
				Labels: []string{"tag1", "tag 2"},
			},
		},
	}})
}

func (s *PayloadRegisterSuite) TestRunErr(c *gc.C) {
	s.hookCtx.SetErrors(errors.New("boo"))
	err := s.runRegisterCommand(c)
	c.Assert(err, gc.ErrorMatches, "boo")
}

func (s *PayloadRegisterSuite) runRegisterCommand(c *gc.C) error {
	ctx := cmdtesting.Context(c)
	return s.command.Run(ctx)
}

type stubPayloadRegisterContext struct {
	jujuc.Context
	*testing.Stub
}

func (f *stubPayloadRegisterContext) TrackPayload(pl params.TrackPayloadParams) error {
	f.AddCall("TrackPayload", pl)
	return f.NextErr()
}
