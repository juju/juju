// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package context_test

import (
	"bytes"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/payload"
	"github.com/juju/juju/payload/context"
	coretesting "github.com/juju/juju/testing"
)

type statusSetSuite struct {
	testing.IsolationSuite

	stub    *testing.Stub
	compCtx *stubSetStatusContext
	ctx     *cmd.Context

	cmd *context.StatusSetCmd
}

var _ = gc.Suite(&statusSetSuite{})

func (s *statusSetSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)

	s.stub = &testing.Stub{}
	s.compCtx = &stubSetStatusContext{stub: s.stub}
	s.ctx = coretesting.Context(c)

	cmd, err := context.NewStatusSetCmd(s)
	c.Assert(err, jc.ErrorIsNil)

	s.cmd = cmd
}

func (s *statusSetSuite) init(c *gc.C, class, id, status string) {
	err := s.cmd.Init([]string{class, id, status})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *statusSetSuite) Component(name string) (context.Component, error) {
	s.stub.AddCall("Component", name)
	if err := s.stub.NextErr(); err != nil {
		return nil, errors.Trace(err)
	}

	return s.compCtx, nil
}

func (s *statusSetSuite) TestHelp(c *gc.C) {
	code := cmd.Main(s.cmd, s.ctx, []string{"--help"})
	c.Assert(code, gc.Equals, 0)

	c.Check(s.ctx.Stdout.(*bytes.Buffer).String(), gc.Equals, `
usage: payload-status-set <class> <id> <status>
purpose: update the status of a payload

"payload-status-set" is used to update the current status of a registered payload.
The <class> and <id> provided must match a payload that has been previously
registered with juju using payload-register. The <status> must be one of the
follow: starting, started, stopping, stopped
`[1:])
}

func (s *statusSetSuite) TestTooFewArgs(c *gc.C) {
	err := s.cmd.Init([]string{})
	c.Check(err, gc.ErrorMatches, `missing .*`)

	err = s.cmd.Init([]string{payload.StateRunning})
	c.Check(err, gc.ErrorMatches, `missing .*`)
}

func (s *statusSetSuite) TestInvalidStatjs(c *gc.C) {
	s.init(c, "docker", "foo", "created")
	err := s.cmd.Run(s.ctx)

	c.Check(err, gc.ErrorMatches, `status .* not supported; expected .*`)
}

func (s *statusSetSuite) TestStatusSet(c *gc.C) {
	s.init(c, "docker", "foo", payload.StateStopped)
	err := s.cmd.Run(s.ctx)

	c.Check(err, jc.ErrorIsNil)
}

type stubSetStatusContext struct {
	context.Component
	stub *testing.Stub
}

func (s stubSetStatusContext) SetStatus(class, id, status string) error {
	s.stub.AddCall("SetStatus", class, id, status)
	if err := s.stub.NextErr(); err != nil {
		return errors.Trace(err)
	}

	return nil
}

func (s stubSetStatusContext) Flush() error {
	s.stub.AddCall("Flush")
	if err := s.stub.NextErr(); err != nil {
		return errors.Trace(err)
	}

	return nil
}
