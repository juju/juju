// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package context_test

import (
	"fmt"

	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	jujuctesting "github.com/juju/juju/worker/uniter/runner/jujuc/testing"
	"github.com/juju/juju/workload"
	"github.com/juju/juju/workload/context"
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

	context.AddWorkloads(s.compCtx, s.workload)
}

func (s *contextSuite) newContext(c *gc.C, workloads ...workload.Info) *context.Context {
	ctx := context.NewContext(s.apiClient, s.dataDir)
	for _, wl := range workloads {
		c.Logf("adding workload: %s", wl.ID())
		context.AddWorkload(ctx, wl.ID(), wl)
	}
	return ctx
}

func (s *contextSuite) TestNewContextEmpty(c *gc.C) {
	ctx := context.NewContext(s.apiClient, s.dataDir)
	workloads, err := ctx.Workloads()
	c.Assert(err, jc.ErrorIsNil)

	c.Check(workloads, gc.HasLen, 0)
}

func (s *contextSuite) TestNewContextPrePopulated(c *gc.C) {
	expected := []workload.Info{
		s.newWorkload("A", "myplugin", "spam", "okay"),
		s.newWorkload("B", "myplugin", "eggs", "okay"),
	}

	ctx := s.newContext(c, expected...)
	workloads, err := ctx.Workloads()
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(workloads, gc.HasLen, 2)

	// Map ordering is indeterminate, so this if-else is needed.
	if workloads[0].Name == "A" {
		c.Check(workloads[0], jc.DeepEquals, expected[0])
		c.Check(workloads[1], jc.DeepEquals, expected[1])
	} else {
		c.Check(workloads[0], jc.DeepEquals, expected[1])
		c.Check(workloads[1], jc.DeepEquals, expected[0])
	}
}

func (s *contextSuite) TestNewContextAPIOkay(c *gc.C) {
	expected := s.apiClient.setNew("A/xyx123")

	ctx, err := context.NewContextAPI(s.apiClient, s.dataDir)
	c.Assert(err, jc.ErrorIsNil)

	workloads, err := ctx.Workloads()
	c.Assert(err, jc.ErrorIsNil)

	c.Check(workloads, jc.DeepEquals, expected)
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

	workloads, err := ctx.Workloads()
	c.Assert(err, jc.ErrorIsNil)

	c.Check(workloads, gc.HasLen, 0)
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
	info.SetComponent(workload.ComponentName, expected)

	compCtx, err := context.ContextComponent(hctx)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(compCtx, gc.Equals, expected)
	s.Stub.CheckCallNames(c, "Component")
	s.Stub.CheckCall(c, 0, "Component", workload.ComponentName)
}

func (s *contextSuite) TestContextComponentMissing(c *gc.C) {
	hctx, _ := s.NewHookContext()
	_, err := context.ContextComponent(hctx)

	c.Check(err, gc.ErrorMatches, fmt.Sprintf("component %q not registered", workload.ComponentName))
	s.Stub.CheckCallNames(c, "Component")
}

func (s *contextSuite) TestContextComponentWrong(c *gc.C) {
	hctx, info := s.NewHookContext()
	compCtx := &jujuctesting.ContextComponent{}
	info.SetComponent(workload.ComponentName, compCtx)

	_, err := context.ContextComponent(hctx)

	c.Check(err, gc.ErrorMatches, "wrong component context type registered: .*")
	s.Stub.CheckCallNames(c, "Component")
}

func (s *contextSuite) TestContextComponentDisabled(c *gc.C) {
	hctx, info := s.NewHookContext()
	info.SetComponent(workload.ComponentName, nil)

	_, err := context.ContextComponent(hctx)

	c.Check(err, gc.ErrorMatches, fmt.Sprintf("component %q disabled", workload.ComponentName))
	s.Stub.CheckCallNames(c, "Component")
}

func (s *contextSuite) TestWorkloadsOkay(c *gc.C) {
	expected := []workload.Info{
		s.newWorkload("A", "myplugin", "spam", "okay"),
		s.newWorkload("B", "myplugin", "eggs", "okay"),
		s.newWorkload("C", "myplugin", "ham", "okay"),
	}

	ctx := s.newContext(c, expected...)
	workloads, err := ctx.Workloads()
	c.Assert(err, jc.ErrorIsNil)

	checkWorkloads(c, workloads, expected)
	s.Stub.CheckCallNames(c)
}

func (s *contextSuite) TestWorkloadsAPI(c *gc.C) {
	expected := s.apiClient.setNew("A/spam", "B/eggs", "C/ham")

	ctx := context.NewContext(s.apiClient, s.dataDir)
	context.AddWorkload(ctx, "A/spam", s.apiClient.workloads["A/spam"])
	context.AddWorkload(ctx, "B/eggs", s.apiClient.workloads["B/eggs"])
	context.AddWorkload(ctx, "C/ham", s.apiClient.workloads["C/ham"])

	workloads, err := ctx.Workloads()
	c.Assert(err, jc.ErrorIsNil)

	checkWorkloads(c, workloads, expected)
	s.Stub.CheckCallNames(c)
}

func (s *contextSuite) TestWorkloadsEmpty(c *gc.C) {
	ctx := context.NewContext(s.apiClient, s.dataDir)
	workloads, err := ctx.Workloads()
	c.Assert(err, jc.ErrorIsNil)

	c.Check(workloads, gc.HasLen, 0)
	s.Stub.CheckCallNames(c)
}

func (s *contextSuite) TestWorkloadsAdditions(c *gc.C) {
	expected := s.apiClient.setNew("A/spam", "B/eggs")
	infoC := s.newWorkload("C", "myplugin", "xyz789", "okay")
	infoD := s.newWorkload("D", "myplugin", "xyzabc", "okay")
	expected = append(expected, infoC, infoD)

	ctx := s.newContext(c, expected[0])
	context.AddWorkload(ctx, "B/eggs", s.apiClient.workloads["B/eggs"])
	ctx.Track(infoC)
	ctx.Track(infoD)

	workloads, err := ctx.Workloads()
	c.Assert(err, jc.ErrorIsNil)

	checkWorkloads(c, workloads, expected)
	s.Stub.CheckCallNames(c)
}

func (s *contextSuite) TestWorkloadsOverrides(c *gc.C) {
	expected := s.apiClient.setNew("A/xyz123", "B/something-else")
	infoB := s.newWorkload("B", "myplugin", "xyz456", "okay")
	infoC := s.newWorkload("C", "myplugin", "xyz789", "okay")
	expected = append(expected[:1], infoB, infoC)

	ctx := context.NewContext(s.apiClient, s.dataDir)
	context.AddWorkload(ctx, "A/xyz123", s.apiClient.workloads["A/xyz123"])
	context.AddWorkload(ctx, "B/xyz456", infoB)
	ctx.Track(infoB)
	ctx.Track(infoC)

	workloads, err := ctx.Workloads()
	c.Assert(err, jc.ErrorIsNil)

	checkWorkloads(c, workloads, expected)
	s.Stub.CheckCallNames(c)
}

func (s *contextSuite) TestGetOkay(c *gc.C) {
	expected := s.newWorkload("A", "myplugin", "spam", "okay")
	extra := s.newWorkload("B", "myplugin", "eggs", "okay")

	ctx := s.newContext(c, expected, extra)
	info, err := ctx.Get("A", "spam")
	c.Assert(err, jc.ErrorIsNil)

	c.Check(*info, jc.DeepEquals, expected)
	s.Stub.CheckCallNames(c)
}

func (s *contextSuite) TestGetOverride(c *gc.C) {
	workloads := s.apiClient.setNew("A/spam", "B/eggs")
	expected := workloads[0]

	unexpected := expected
	unexpected.Details.ID = "C"

	ctx := s.newContext(c, workloads[1])
	context.AddWorkload(ctx, "A/spam", unexpected)
	context.AddWorkload(ctx, "A/spam", expected)

	info, err := ctx.Get("A", "spam")
	c.Assert(err, jc.ErrorIsNil)

	c.Check(*info, jc.DeepEquals, expected)
	s.Stub.CheckCallNames(c)
}

func (s *contextSuite) TestGetNotFound(c *gc.C) {
	ctx := context.NewContext(s.apiClient, s.dataDir)
	_, err := ctx.Get("A", "spam")

	c.Check(err, jc.Satisfies, errors.IsNotFound)
}

func (s *contextSuite) TestSetOkay(c *gc.C) {
	info := s.newWorkload("A", "myplugin", "spam", "okay")
	ctx := context.NewContext(s.apiClient, s.dataDir)
	before, err := ctx.Workloads()
	c.Assert(err, jc.ErrorIsNil)
	err = ctx.Track(info)
	c.Assert(err, jc.ErrorIsNil)
	after, err := ctx.Workloads()
	c.Assert(err, jc.ErrorIsNil)

	c.Check(before, gc.HasLen, 0)
	c.Check(after, jc.DeepEquals, []workload.Info{info})
}

func (s *contextSuite) TestSetOverwrite(c *gc.C) {
	info := s.newWorkload("A", "myplugin", "xyz123", "running")
	other := s.newWorkload("A", "myplugin", "xyz123", "stopped")
	ctx := s.newContext(c, other)
	before, err := ctx.Workloads()
	c.Assert(err, jc.ErrorIsNil)
	err = ctx.Track(info)
	c.Assert(err, jc.ErrorIsNil)
	after, err := ctx.Workloads()
	c.Assert(err, jc.ErrorIsNil)

	c.Check(before, jc.DeepEquals, []workload.Info{other})
	c.Check(after, jc.DeepEquals, []workload.Info{info})
}

func (s *contextSuite) TestFlushNotDirty(c *gc.C) {
	info := s.newWorkload("flush-not-dirty", "myplugin", "xyz123", "okay")
	ctx := s.newContext(c, info)

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
	info := s.newWorkload("A", "myplugin", "spam", "okay")
	ctx := context.NewContext(s.apiClient, s.dataDir)
	err := ctx.Track(info)
	c.Assert(err, jc.ErrorIsNil)
	before, err := ctx.Workloads()
	c.Assert(err, jc.ErrorIsNil)
	c.Check(before, jc.DeepEquals, []workload.Info{info})
	ctx.Untrack("uh-oh", "not gonna match")
	after, err := ctx.Workloads()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(after, gc.DeepEquals, before)
}
