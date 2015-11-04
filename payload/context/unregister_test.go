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

type unregisterSuite struct {
	testing.IsolationSuite

	stub    *testing.Stub
	compCtx *stubUnregisterContext
	ctx     *cmd.Context
}

var _ = gc.Suite(&unregisterSuite{})

func (s *unregisterSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)

	s.stub = &testing.Stub{}
	s.compCtx = &stubUnregisterContext{stub: s.stub}
	s.ctx = coretesting.Context(c)
}

func (s *unregisterSuite) Component(name string) (context.Component, error) {
	s.stub.AddCall("Component", name)
	if err := s.stub.NextErr(); err != nil {
		return nil, errors.Trace(err)
	}

	return s.compCtx, nil
}

func (s *unregisterSuite) TestHelp(c *gc.C) {
	unregister, err := context.NewUnregisterCmd(s)
	c.Assert(err, jc.ErrorIsNil)
	code := cmd.Main(unregister, s.ctx, []string{"--help"})
	c.Assert(code, gc.Equals, 0)

	c.Check(s.ctx.Stdout.(*bytes.Buffer).String(), gc.Equals, `
usage: payload-unregister <class> <id>
purpose: stop tracking a payload

"payload-unregister" is used while a hook is running to let Juju know
that a payload has been manually stopped. The <class> and <id> provided
must match a payload that has been previously registered with juju using
payload-register.
`[1:])
}

func (s *unregisterSuite) TestRunOkay(c *gc.C) {
	unregister, err := context.NewUnregisterCmd(s)
	c.Assert(err, jc.ErrorIsNil)
	err = unregister.Init([]string{"spam", "eggs"})
	c.Assert(err, jc.ErrorIsNil)
	err = unregister.Run(s.ctx)
	c.Assert(err, jc.ErrorIsNil)

	s.stub.CheckCalls(c, []testing.StubCall{{
		FuncName: "Component",
		Args: []interface{}{
			payload.ComponentName,
		},
	}, {
		FuncName: "Untrack",
		Args: []interface{}{
			"spam",
			"eggs",
		},
	}, {
		FuncName: "Flush",
	}})
}

type stubUnregisterContext struct {
	context.Component
	stub *testing.Stub
}

func (s stubUnregisterContext) Untrack(class, id string) error {
	s.stub.AddCall("Untrack", class, id)
	if err := s.stub.NextErr(); err != nil {
		return errors.Trace(err)
	}

	return nil
}

func (s stubUnregisterContext) Flush() error {
	s.stub.AddCall("Flush")
	if err := s.stub.NextErr(); err != nil {
		return errors.Trace(err)
	}

	return nil
}
