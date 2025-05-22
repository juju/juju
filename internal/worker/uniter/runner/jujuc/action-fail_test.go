// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package jujuc_test

import (
	"fmt"
	"testing"

	"github.com/juju/tc"

	"github.com/juju/juju/internal/cmd"
	"github.com/juju/juju/internal/cmd/cmdtesting"
	"github.com/juju/juju/internal/worker/uniter/runner/jujuc"
)

type ActionFailSuite struct {
	ContextSuite
}

type actionFailContext struct {
	jujuc.Context
	actionFailed  bool
	actionMessage string
}

func (ctx *actionFailContext) SetActionMessage(message string) error {
	ctx.actionMessage = message
	return nil
}

func (ctx *actionFailContext) SetActionFailed() error {
	ctx.actionFailed = true
	return nil
}

type nonActionFailContext struct {
	jujuc.Context
}

func (ctx *nonActionFailContext) SetActionMessage(message string) error {
	return fmt.Errorf("not running an action")
}

func (ctx *nonActionFailContext) SetActionFailed() error {
	return fmt.Errorf("not running an action")
}
func TestActionFailSuite(t *testing.T) {
	tc.Run(t, &ActionFailSuite{})
}

func (s *ActionFailSuite) TestActionFail(c *tc.C) {
	var actionFailTests = []struct {
		summary string
		command []string
		message string
		failed  bool
		errMsg  string
		code    int
	}{{
		summary: "no parameters sets a default message",
		command: []string{},
		message: "action failed without reason given, check action for errors",
		failed:  true,
	}, {
		summary: "a message sent is set as the failure reason",
		command: []string{"a failure message"},
		message: "a failure message",
		failed:  true,
	}, {
		summary: "extra arguments are an error, leaving the action not failed",
		command: []string{"a failure message", "something else"},
		errMsg:  "ERROR unrecognized args: [\"something else\"]\n",
		code:    2,
	}}

	for i, t := range actionFailTests {
		c.Logf("test %d: %s", i, t.summary)
		hctx := &actionFailContext{}
		com, err := jujuc.NewCommand(hctx, "action-fail")
		c.Assert(err, tc.ErrorIsNil)
		ctx := cmdtesting.Context(c)
		code := cmd.Main(jujuc.NewJujucCommandWrappedForTest(com), ctx, t.command)
		c.Check(code, tc.Equals, t.code)
		c.Check(bufferString(ctx.Stderr), tc.Equals, t.errMsg)
		c.Check(hctx.actionMessage, tc.Equals, t.message)
		c.Check(hctx.actionFailed, tc.Equals, t.failed)
	}
}

func (s *ActionFailSuite) TestNonActionSetActionFailedFails(c *tc.C) {
	hctx := &nonActionFailContext{}
	com, err := jujuc.NewCommand(hctx, "action-fail")
	c.Assert(err, tc.ErrorIsNil)
	ctx := cmdtesting.Context(c)
	code := cmd.Main(jujuc.NewJujucCommandWrappedForTest(com), ctx, []string{"oops"})
	c.Check(code, tc.Equals, 1)
	c.Check(bufferString(ctx.Stderr), tc.Equals, "ERROR not running an action\n")
	c.Check(bufferString(ctx.Stdout), tc.Equals, "")
}
