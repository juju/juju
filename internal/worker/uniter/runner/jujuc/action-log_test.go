// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package jujuc_test

import (
	"context"
	"fmt"

	"github.com/juju/cmd/v4"
	"github.com/juju/cmd/v4/cmdtesting"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/internal/worker/uniter/runner/jujuc"
)

type ActionLogSuite struct {
	ContextSuite
}

type actionLogContext struct {
	jujuc.Context
	logMessage string
}

func (ctx *actionLogContext) LogActionMessage(_ context.Context, message string) error {
	ctx.logMessage = message
	return nil
}

type nonActionLogContext struct {
	jujuc.Context
}

func (ctx *nonActionLogContext) LogActionMessage(_ context.Context, message string) error {
	return fmt.Errorf("not running an action")
}

var _ = gc.Suite(&ActionLogSuite{})

func (s *ActionLogSuite) TestActionLog(c *gc.C) {
	var actionLogTests = []struct {
		summary string
		command []string
		message string
		code    int
		errMsg  string
	}{{
		summary: "log message as a single argument",
		command: []string{"a failure message"},
		message: "a failure message",
	}, {
		summary: "more than one arguments are concatenated",
		command: []string{"a log message", "something else"},
		message: "a log message something else",
	}, {
		summary: "no message specified",
		command: []string{},
		errMsg:  "ERROR no message specified\n",
		code:    2,
	}}

	for i, t := range actionLogTests {
		c.Logf("test %d: %s", i, t.summary)
		hctx := &actionLogContext{}
		com, err := jujuc.NewCommand(hctx, "action-log")
		c.Assert(err, jc.ErrorIsNil)
		ctx := cmdtesting.Context(c)
		code := cmd.Main(jujuc.NewJujucCommandWrappedForTest(com), ctx, t.command)
		c.Check(code, gc.Equals, t.code)
		c.Check(bufferString(ctx.Stderr), gc.Equals, t.errMsg)
		c.Check(hctx.logMessage, gc.Equals, t.message)
	}
}

func (s *ActionLogSuite) TestNonActionLogActionFails(c *gc.C) {
	hctx := &nonActionLogContext{}
	com, err := jujuc.NewCommand(hctx, "action-log")
	c.Assert(err, jc.ErrorIsNil)
	ctx := cmdtesting.Context(c)
	code := cmd.Main(jujuc.NewJujucCommandWrappedForTest(com), ctx, []string{"oops"})
	c.Check(code, gc.Equals, 1)
	c.Check(bufferString(ctx.Stderr), gc.Equals, "ERROR not running an action\n")
	c.Check(bufferString(ctx.Stdout), gc.Equals, "")
}
