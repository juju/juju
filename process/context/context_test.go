// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package context_test

import (
	"fmt"

	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/charm.v5"

	"github.com/juju/juju/process"
	"github.com/juju/juju/process/context"
	jujuctesting "github.com/juju/juju/worker/uniter/runner/jujuc/testing"
)

type contextSuite struct {
	baseSuite
	compCtx   *context.Context
	apiClient *stubAPIClient
}

var _ = gc.Suite(&contextSuite{})

func (s *contextSuite) SetUpTest(c *gc.C) {
	s.baseSuite.SetUpTest(c)

	s.apiClient = newStubAPIClient(s.Stub)
	s.compCtx = context.NewContext(s.apiClient)

	context.AddProcs(s.compCtx, s.proc)
}

func (s *contextSuite) newContext(c *gc.C, procs ...process.Info) *context.Context {
	ctx := context.NewContext(s.apiClient)
	for _, proc := range procs {
		c.Logf("adding proc: %s", proc.ID())
		context.AddProc(ctx, proc.ID(), proc)
	}
	return ctx
}

func (s *contextSuite) TestNewContextEmpty(c *gc.C) {
	ctx := context.NewContext(s.apiClient)
	procs, err := ctx.Processes()
	c.Assert(err, jc.ErrorIsNil)

	c.Check(procs, gc.HasLen, 0)
}

func (s *contextSuite) TestNewContextPrePopulated(c *gc.C) {
	expected := []process.Info{
		s.newProc("A", "myplugin", "spam", "okay"),
		s.newProc("B", "myplugin", "eggs", "okay"),
	}

	ctx := s.newContext(c, expected...)
	procs, err := ctx.Processes()
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(procs, gc.HasLen, 2)

	// Map ordering is indeterminate, so this if-else is needed.
	if procs[0].Name == "A" {
		c.Check(procs[0], jc.DeepEquals, expected[0])
		c.Check(procs[1], jc.DeepEquals, expected[1])
	} else {
		c.Check(procs[0], jc.DeepEquals, expected[1])
		c.Check(procs[1], jc.DeepEquals, expected[0])
	}
}

func (s *contextSuite) TestNewContextAPIOkay(c *gc.C) {
	expected := s.apiClient.setNew("A/xyx123")

	ctx, err := context.NewContextAPI(s.apiClient)
	c.Assert(err, jc.ErrorIsNil)

	procs, err := ctx.Processes()
	c.Assert(err, jc.ErrorIsNil)

	c.Check(procs, jc.DeepEquals, expected)
}

func (s *contextSuite) TestNewContextAPICalls(c *gc.C) {
	s.apiClient.setNew("A/xyz123")

	_, err := context.NewContextAPI(s.apiClient)
	c.Assert(err, jc.ErrorIsNil)

	s.Stub.CheckCallNames(c, "ListProcesses")
}

func (s *contextSuite) TestNewContextAPIEmpty(c *gc.C) {
	ctx, err := context.NewContextAPI(s.apiClient)
	c.Assert(err, jc.ErrorIsNil)

	procs, err := ctx.Processes()
	c.Assert(err, jc.ErrorIsNil)

	c.Check(procs, gc.HasLen, 0)
}

func (s *contextSuite) TestNewContextAPIError(c *gc.C) {
	expected := errors.Errorf("<failed>")
	s.Stub.SetErrors(expected)

	_, err := context.NewContextAPI(s.apiClient)

	c.Check(errors.Cause(err), gc.Equals, expected)
	s.Stub.CheckCallNames(c, "ListProcesses")
}

func (s *contextSuite) TestContextComponentOkay(c *gc.C) {
	hctx, info := s.NewHookContext()
	expected := context.NewContext(s.apiClient)
	info.SetComponent(process.ComponentName, expected)

	compCtx, err := context.ContextComponent(hctx)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(compCtx, gc.Equals, expected)
	s.Stub.CheckCallNames(c, "Component")
	s.Stub.CheckCall(c, 0, "Component", process.ComponentName)
}

func (s *contextSuite) TestContextComponentMissing(c *gc.C) {
	hctx, _ := s.NewHookContext()
	_, err := context.ContextComponent(hctx)

	c.Check(err, gc.ErrorMatches, fmt.Sprintf("component %q not registered", process.ComponentName))
	s.Stub.CheckCallNames(c, "Component")
}

func (s *contextSuite) TestContextComponentWrong(c *gc.C) {
	hctx, info := s.NewHookContext()
	compCtx := &jujuctesting.ContextComponent{}
	info.SetComponent(process.ComponentName, compCtx)

	_, err := context.ContextComponent(hctx)

	c.Check(err, gc.ErrorMatches, "wrong component context type registered: .*")
	s.Stub.CheckCallNames(c, "Component")
}

func (s *contextSuite) TestContextComponentDisabled(c *gc.C) {
	hctx, info := s.NewHookContext()
	info.SetComponent(process.ComponentName, nil)

	_, err := context.ContextComponent(hctx)

	c.Check(err, gc.ErrorMatches, fmt.Sprintf("component %q disabled", process.ComponentName))
	s.Stub.CheckCallNames(c, "Component")
}

func (s *contextSuite) TestProcessesOkay(c *gc.C) {
	expected := []process.Info{
		s.newProc("A", "myplugin", "spam", "okay"),
		s.newProc("B", "myplugin", "eggs", "okay"),
		s.newProc("C", "myplugin", "ham", "okay"),
	}

	ctx := s.newContext(c, expected...)
	procs, err := ctx.Processes()
	c.Assert(err, jc.ErrorIsNil)

	checkProcs(c, procs, expected)
	s.Stub.CheckCallNames(c)
}

func (s *contextSuite) TestProcessesAPI(c *gc.C) {
	expected := s.apiClient.setNew("A/spam", "B/eggs", "C/ham")

	ctx := context.NewContext(s.apiClient)
	context.AddProc(ctx, "A/spam", s.apiClient.procs["A/spam"])
	context.AddProc(ctx, "B/eggs", s.apiClient.procs["B/eggs"])
	context.AddProc(ctx, "C/ham", s.apiClient.procs["C/ham"])

	procs, err := ctx.Processes()
	c.Assert(err, jc.ErrorIsNil)

	checkProcs(c, procs, expected)
	s.Stub.CheckCallNames(c)
}

func (s *contextSuite) TestProcessesEmpty(c *gc.C) {
	ctx := context.NewContext(s.apiClient)
	procs, err := ctx.Processes()
	c.Assert(err, jc.ErrorIsNil)

	c.Check(procs, gc.HasLen, 0)
	s.Stub.CheckCallNames(c)
}

func (s *contextSuite) TestProcessesAdditions(c *gc.C) {
	expected := s.apiClient.setNew("A/spam", "B/eggs")
	infoC := s.newProc("C", "myplugin", "xyz789", "okay")
	infoD := s.newProc("D", "myplugin", "xyzabc", "okay")
	expected = append(expected, infoC, infoD)

	ctx := s.newContext(c, expected[0])
	context.AddProc(ctx, "B/eggs", s.apiClient.procs["B/eggs"])
	ctx.Set(infoC)
	ctx.Set(infoD)

	procs, err := ctx.Processes()
	c.Assert(err, jc.ErrorIsNil)

	checkProcs(c, procs, expected)
	s.Stub.CheckCallNames(c)
}

func (s *contextSuite) TestProcessesOverrides(c *gc.C) {
	expected := s.apiClient.setNew("A/xyz123", "B/something-else")
	infoB := s.newProc("B", "myplugin", "xyz456", "okay")
	infoC := s.newProc("C", "myplugin", "xyz789", "okay")
	expected = append(expected[:1], infoB, infoC)

	ctx := context.NewContext(s.apiClient)
	context.AddProc(ctx, "A/xyz123", s.apiClient.procs["A/xyz123"])
	context.AddProc(ctx, "B/xyz456", infoB)
	ctx.Set(infoB)
	ctx.Set(infoC)

	procs, err := ctx.Processes()
	c.Assert(err, jc.ErrorIsNil)

	checkProcs(c, procs, expected)
	s.Stub.CheckCallNames(c)
}

func (s *contextSuite) TestGetOkay(c *gc.C) {
	expected := s.newProc("A", "myplugin", "spam", "okay")
	extra := s.newProc("B", "myplugin", "eggs", "okay")

	ctx := s.newContext(c, expected, extra)
	info, err := ctx.Get("A/spam")
	c.Assert(err, jc.ErrorIsNil)

	c.Check(*info, jc.DeepEquals, expected)
	s.Stub.CheckCallNames(c)
}

func (s *contextSuite) TestGetOverride(c *gc.C) {
	procs := s.apiClient.setNew("A/spam", "B/eggs")
	expected := procs[0]

	unexpected := expected
	unexpected.Details.ID = "C"

	ctx := s.newContext(c, procs[1])
	context.AddProc(ctx, "A/spam", unexpected)
	context.AddProc(ctx, "A/spam", expected)

	info, err := ctx.Get("A/spam")
	c.Assert(err, jc.ErrorIsNil)

	c.Check(*info, jc.DeepEquals, expected)
	s.Stub.CheckCallNames(c)
}

func (s *contextSuite) TestGetNotFound(c *gc.C) {
	ctx := context.NewContext(s.apiClient)
	_, err := ctx.Get("A/spam")

	c.Check(err, jc.Satisfies, errors.IsNotFound)
}

func (s *contextSuite) TestSetOkay(c *gc.C) {
	info := s.newProc("A", "myplugin", "spam", "okay")
	ctx := context.NewContext(s.apiClient)
	before, err := ctx.Processes()
	c.Assert(err, jc.ErrorIsNil)
	err = ctx.Set(info)
	c.Assert(err, jc.ErrorIsNil)
	after, err := ctx.Processes()
	c.Assert(err, jc.ErrorIsNil)

	c.Check(before, gc.HasLen, 0)
	c.Check(after, jc.DeepEquals, []process.Info{info})
}

func (s *contextSuite) TestSetOverwrite(c *gc.C) {
	info := s.newProc("A", "myplugin", "xyz123", "running")
	other := s.newProc("A", "myplugin", "xyz123", "stopped")
	ctx := s.newContext(c, other)
	before, err := ctx.Processes()
	c.Assert(err, jc.ErrorIsNil)
	err = ctx.Set(info)
	c.Assert(err, jc.ErrorIsNil)
	after, err := ctx.Processes()
	c.Assert(err, jc.ErrorIsNil)

	c.Check(before, jc.DeepEquals, []process.Info{other})
	c.Check(after, jc.DeepEquals, []process.Info{info})
}

func (s *contextSuite) TestListDefinitions(c *gc.C) {
	definition := charm.Process{
		Name: "procA",
		Type: "myplugin",
	}
	s.apiClient.definitions["procA"] = definition
	ctx := context.NewContext(s.apiClient)

	definitions, err := ctx.ListDefinitions()
	c.Assert(err, jc.ErrorIsNil)

	c.Check(definitions, gc.DeepEquals, []charm.Process{
		definition,
	})
	s.Stub.CheckCallNames(c, "AllDefinitions")
}

func (s *contextSuite) TestFlushDirty(c *gc.C) {
	info := s.newProc("A", "myplugin", "xyz123", "okay")

	ctx := context.NewContext(s.apiClient)
	err := ctx.Set(info)
	c.Assert(err, jc.ErrorIsNil)

	err = ctx.Flush()
	c.Assert(err, jc.ErrorIsNil)

	s.Stub.CheckCallNames(c, "RegisterProcesses")
}

func (s *contextSuite) TestFlushNotDirty(c *gc.C) {
	info := s.newProc("flush-not-dirty", "myplugin", "xyz123", "okay")
	ctx := s.newContext(c, info)

	err := ctx.Flush()
	c.Assert(err, jc.ErrorIsNil)

	s.Stub.CheckCallNames(c)
}

func (s *contextSuite) TestFlushEmpty(c *gc.C) {
	ctx := context.NewContext(s.apiClient)
	err := ctx.Flush()
	c.Assert(err, jc.ErrorIsNil)

	s.Stub.CheckCallNames(c)
}
