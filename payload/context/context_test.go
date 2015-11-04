// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package context_test

import (
	"fmt"

	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/payload"
	"github.com/juju/juju/payload/context"
	jujuctesting "github.com/juju/juju/worker/uniter/runner/jujuc/testing"
)

type contextSuite struct {
	baseSuite
	compCtx   *context.Context
	apiClient *stubAPIClient
	dataDir   string
}

var _ = gc.Suite(&contextSuite{})

func (s *contextSuite) SetUpTest(c *gc.C) {
	s.baseSuite.SetUpTest(c)

	s.apiClient = newStubAPIClient(s.Stub)
	s.dataDir = "some-data-dir"
	s.compCtx = context.NewContext(s.apiClient, s.dataDir)

	context.AddPayloads(s.compCtx, s.payload)
}

func (s *contextSuite) newContext(c *gc.C, payloads ...payload.Payload) *context.Context {
	ctx := context.NewContext(s.apiClient, s.dataDir)
	for _, pl := range payloads {
		c.Logf("adding payload: %s", pl.FullID())
		context.AddPayload(ctx, pl.FullID(), pl)
	}
	return ctx
}

func (s *contextSuite) TestNewContextEmpty(c *gc.C) {
	ctx := context.NewContext(s.apiClient, s.dataDir)
	payloads, err := ctx.Payloads()
	c.Assert(err, jc.ErrorIsNil)

	c.Check(payloads, gc.HasLen, 0)
}

func (s *contextSuite) TestNewContextPrePopulated(c *gc.C) {
	expected := []payload.Payload{
		s.newPayload("A", "myplugin", "spam", "running"),
		s.newPayload("B", "myplugin", "eggs", "running"),
	}

	ctx := s.newContext(c, expected...)
	payloads, err := ctx.Payloads()
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(payloads, gc.HasLen, 2)

	// Map ordering is indeterminate, so this if-else is needed.
	if payloads[0].Name == "A" {
		c.Check(payloads[0], jc.DeepEquals, expected[0])
		c.Check(payloads[1], jc.DeepEquals, expected[1])
	} else {
		c.Check(payloads[0], jc.DeepEquals, expected[1])
		c.Check(payloads[1], jc.DeepEquals, expected[0])
	}
}

func (s *contextSuite) TestNewContextAPIOkay(c *gc.C) {
	expected := s.apiClient.setNew("A/xyx123")

	ctx, err := context.NewContextAPI(s.apiClient, s.dataDir)
	c.Assert(err, jc.ErrorIsNil)

	payloads, err := ctx.Payloads()
	c.Assert(err, jc.ErrorIsNil)

	c.Check(payloads, jc.DeepEquals, expected)
}

func (s *contextSuite) TestNewContextAPICalls(c *gc.C) {
	s.apiClient.setNew("A/xyz123")

	_, err := context.NewContextAPI(s.apiClient, s.dataDir)
	c.Assert(err, jc.ErrorIsNil)

	s.Stub.CheckCallNames(c, "List")
}

func (s *contextSuite) TestNewContextAPIEmpty(c *gc.C) {
	ctx, err := context.NewContextAPI(s.apiClient, s.dataDir)
	c.Assert(err, jc.ErrorIsNil)

	payloads, err := ctx.Payloads()
	c.Assert(err, jc.ErrorIsNil)

	c.Check(payloads, gc.HasLen, 0)
}

func (s *contextSuite) TestNewContextAPIError(c *gc.C) {
	expected := errors.Errorf("<failed>")
	s.Stub.SetErrors(expected)

	_, err := context.NewContextAPI(s.apiClient, s.dataDir)

	c.Check(errors.Cause(err), gc.Equals, expected)
	s.Stub.CheckCallNames(c, "List")
}

func (s *contextSuite) TestContextComponentOkay(c *gc.C) {
	hctx, info := s.NewHookContext()
	expected := context.NewContext(s.apiClient, s.dataDir)
	info.SetComponent(payload.ComponentName, expected)

	compCtx, err := context.ContextComponent(hctx)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(compCtx, gc.Equals, expected)
	s.Stub.CheckCallNames(c, "Component")
	s.Stub.CheckCall(c, 0, "Component", payload.ComponentName)
}

func (s *contextSuite) TestContextComponentMissing(c *gc.C) {
	hctx, _ := s.NewHookContext()
	_, err := context.ContextComponent(hctx)

	c.Check(err, gc.ErrorMatches, fmt.Sprintf("component %q not registered", payload.ComponentName))
	s.Stub.CheckCallNames(c, "Component")
}

func (s *contextSuite) TestContextComponentWrong(c *gc.C) {
	hctx, info := s.NewHookContext()
	compCtx := &jujuctesting.ContextComponent{}
	info.SetComponent(payload.ComponentName, compCtx)

	_, err := context.ContextComponent(hctx)

	c.Check(err, gc.ErrorMatches, "wrong component context type registered: .*")
	s.Stub.CheckCallNames(c, "Component")
}

func (s *contextSuite) TestContextComponentDisabled(c *gc.C) {
	hctx, info := s.NewHookContext()
	info.SetComponent(payload.ComponentName, nil)

	_, err := context.ContextComponent(hctx)

	c.Check(err, gc.ErrorMatches, fmt.Sprintf("component %q disabled", payload.ComponentName))
	s.Stub.CheckCallNames(c, "Component")
}

func (s *contextSuite) TestPayloadsOkay(c *gc.C) {
	expected := []payload.Payload{
		s.newPayload("A", "myplugin", "spam", "running"),
		s.newPayload("B", "myplugin", "eggs", "running"),
		s.newPayload("C", "myplugin", "ham", "running"),
	}

	ctx := s.newContext(c, expected...)
	payloads, err := ctx.Payloads()
	c.Assert(err, jc.ErrorIsNil)

	checkPayloads(c, payloads, expected)
	s.Stub.CheckCallNames(c)
}

func (s *contextSuite) TestPayloadsAPI(c *gc.C) {
	expected := s.apiClient.setNew("A/spam", "B/eggs", "C/ham")

	ctx := context.NewContext(s.apiClient, s.dataDir)
	context.AddPayload(ctx, "A/spam", s.apiClient.payloads["A/spam"])
	context.AddPayload(ctx, "B/eggs", s.apiClient.payloads["B/eggs"])
	context.AddPayload(ctx, "C/ham", s.apiClient.payloads["C/ham"])

	payloads, err := ctx.Payloads()
	c.Assert(err, jc.ErrorIsNil)

	checkPayloads(c, payloads, expected)
	s.Stub.CheckCallNames(c)
}

func (s *contextSuite) TestPayloadsEmpty(c *gc.C) {
	ctx := context.NewContext(s.apiClient, s.dataDir)
	payloads, err := ctx.Payloads()
	c.Assert(err, jc.ErrorIsNil)

	c.Check(payloads, gc.HasLen, 0)
	s.Stub.CheckCallNames(c)
}

func (s *contextSuite) TestPayloadsAdditions(c *gc.C) {
	expected := s.apiClient.setNew("A/spam", "B/eggs")
	plC := s.newPayload("C", "myplugin", "xyz789", "running")
	plD := s.newPayload("D", "myplugin", "xyzabc", "running")
	expected = append(expected, plC, plD)

	ctx := s.newContext(c, expected[0])
	context.AddPayload(ctx, "B/eggs", s.apiClient.payloads["B/eggs"])
	ctx.Track(plC)
	ctx.Track(plD)

	payloads, err := ctx.Payloads()
	c.Assert(err, jc.ErrorIsNil)

	checkPayloads(c, payloads, expected)
	s.Stub.CheckCallNames(c)
}

func (s *contextSuite) TestPayloadsOverrides(c *gc.C) {
	expected := s.apiClient.setNew("A/xyz123", "B/something-else")
	plB := s.newPayload("B", "myplugin", "xyz456", "running")
	plC := s.newPayload("C", "myplugin", "xyz789", "running")
	expected = append(expected[:1], plB, plC)

	ctx := context.NewContext(s.apiClient, s.dataDir)
	context.AddPayload(ctx, "A/xyz123", s.apiClient.payloads["A/xyz123"])
	context.AddPayload(ctx, "B/xyz456", plB)
	ctx.Track(plB)
	ctx.Track(plC)

	payloads, err := ctx.Payloads()
	c.Assert(err, jc.ErrorIsNil)

	checkPayloads(c, payloads, expected)
	s.Stub.CheckCallNames(c)
}

func (s *contextSuite) TestGetOkay(c *gc.C) {
	expected := s.newPayload("A", "myplugin", "spam", "running")
	extra := s.newPayload("B", "myplugin", "eggs", "running")

	ctx := s.newContext(c, expected, extra)
	pl, err := ctx.Get("A", "spam")
	c.Assert(err, jc.ErrorIsNil)

	c.Check(*pl, jc.DeepEquals, expected)
	s.Stub.CheckCallNames(c)
}

func (s *contextSuite) TestGetOverride(c *gc.C) {
	payloads := s.apiClient.setNew("A/spam", "B/eggs")
	expected := payloads[0]

	unexpected := expected
	unexpected.ID = "C"

	ctx := s.newContext(c, payloads[1])
	context.AddPayload(ctx, "A/spam", unexpected)
	context.AddPayload(ctx, "A/spam", expected)

	pl, err := ctx.Get("A", "spam")
	c.Assert(err, jc.ErrorIsNil)

	c.Check(*pl, jc.DeepEquals, expected)
	s.Stub.CheckCallNames(c)
}

func (s *contextSuite) TestGetNotFound(c *gc.C) {
	ctx := context.NewContext(s.apiClient, s.dataDir)
	_, err := ctx.Get("A", "spam")

	c.Check(err, jc.Satisfies, errors.IsNotFound)
}

func (s *contextSuite) TestSetOkay(c *gc.C) {
	pl := s.newPayload("A", "myplugin", "spam", "running")
	ctx := context.NewContext(s.apiClient, s.dataDir)
	before, err := ctx.Payloads()
	c.Assert(err, jc.ErrorIsNil)
	err = ctx.Track(pl)
	c.Assert(err, jc.ErrorIsNil)
	after, err := ctx.Payloads()
	c.Assert(err, jc.ErrorIsNil)

	c.Check(before, gc.HasLen, 0)
	c.Check(after, jc.DeepEquals, []payload.Payload{pl})
}

func (s *contextSuite) TestSetOverwrite(c *gc.C) {
	pl := s.newPayload("A", "myplugin", "xyz123", "running")
	other := s.newPayload("A", "myplugin", "xyz123", "stopped")
	ctx := s.newContext(c, other)
	before, err := ctx.Payloads()
	c.Assert(err, jc.ErrorIsNil)
	err = ctx.Track(pl)
	c.Assert(err, jc.ErrorIsNil)
	after, err := ctx.Payloads()
	c.Assert(err, jc.ErrorIsNil)

	c.Check(before, jc.DeepEquals, []payload.Payload{other})
	c.Check(after, jc.DeepEquals, []payload.Payload{pl})
}

func (s *contextSuite) TestFlushNotDirty(c *gc.C) {
	pl := s.newPayload("flush-not-dirty", "myplugin", "xyz123", "running")
	ctx := s.newContext(c, pl)

	err := ctx.Flush()
	c.Assert(err, jc.ErrorIsNil)

	s.Stub.CheckCallNames(c)
}

func (s *contextSuite) TestFlushEmpty(c *gc.C) {
	ctx := context.NewContext(s.apiClient, s.dataDir)
	err := ctx.Flush()
	c.Assert(err, jc.ErrorIsNil)

	s.Stub.CheckCallNames(c)
}

func (s *contextSuite) TestUntrackNoMatch(c *gc.C) {
	pl := s.newPayload("A", "myplugin", "spam", "running")
	ctx := context.NewContext(s.apiClient, s.dataDir)
	err := ctx.Track(pl)
	c.Assert(err, jc.ErrorIsNil)
	before, err := ctx.Payloads()
	c.Assert(err, jc.ErrorIsNil)
	c.Check(before, jc.DeepEquals, []payload.Payload{pl})
	ctx.Untrack("uh-oh", "not gonna match")
	after, err := ctx.Payloads()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(after, gc.DeepEquals, before)
}
