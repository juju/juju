// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package context_test

import (
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/process"
	"github.com/juju/juju/process/context"
	jujuctesting "github.com/juju/juju/worker/uniter/runner/jujuc/testing"
)

type baseSuite struct {
	jujuctesting.ContextSuite
	proc    *process.Info
	compCtx *context.Context
	//compCtx *jujuctesting.ContextComponent
}

func (s *baseSuite) SetUpTest(c *gc.C) {
	s.ContextSuite.SetUpTest(c)

	proc := process.NewInfo("proc A", "docker")
	compCtx := context.NewContext(proc)
	//compCtx := &jujuctesting.ContextComponent{Stub: s.Stub}

	s.Ctx = s.HookContext("u/0", nil)
	s.Ctx.SetComponent(process.ComponentName, compCtx)

	s.proc = proc
	s.compCtx = compCtx
	//s.compCtx = compCtx
}

type contextSuite struct {
	baseSuite
}

var _ = gc.Suite(&contextSuite{})

func (s *contextSuite) TestNewContextEmpty(c *gc.C) {
	ctx := context.NewContext()
	procs := ctx.Processes()

	c.Check(procs, gc.HasLen, 0)
}

func (s *contextSuite) TestNewContextPrePopulated(c *gc.C) {
	expected := []*process.Info{{}, {}}
	expected[0].Name = "A"
	expected[1].Name = "B"

	ctx := context.NewContext(expected...)
	procs := ctx.Processes()

	c.Assert(procs, gc.HasLen, 2)
	if procs[0].Name == "A" {
		c.Check(procs[0], jc.DeepEquals, expected[0])
		c.Check(procs[1], jc.DeepEquals, expected[1])
	} else {
		c.Check(procs[0], jc.DeepEquals, expected[1])
		c.Check(procs[1], jc.DeepEquals, expected[0])
	}
}

func (s *contextSuite) TestContextComponentOkay(c *gc.C) {
	ctx := s.HookContext("u/1", nil)
	expected := context.NewContext()
	ctx.SetComponent(process.ComponentName, expected)

	compCtx, err := context.ContextComponent(ctx.Context)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(compCtx, gc.Equals, expected)
	s.Stub.CheckCallNames(c, "Component")
	s.Stub.CheckCall(c, 0, "Component", "process")
}

func (s *contextSuite) TestContextComponentMissing(c *gc.C) {
	ctx := s.HookContext("u/1", nil)
	_, err := context.ContextComponent(ctx.Context)

	c.Check(err, gc.ErrorMatches, `component "process" not registered`)
	s.Stub.CheckCallNames(c, "Component")
}

func (s *contextSuite) TestContextComponentWrong(c *gc.C) {
	ctx := s.HookContext("u/1", nil)
	compCtx := &jujuctesting.ContextComponent{}
	ctx.SetComponent(process.ComponentName, compCtx)

	_, err := context.ContextComponent(ctx.Context)

	c.Check(err, gc.ErrorMatches, "wrong component context type registered: .*")
	s.Stub.CheckCallNames(c, "Component")
}

func (s *contextSuite) TestContextComponentDisabled(c *gc.C) {
	ctx := s.HookContext("u/1", nil)
	ctx.SetComponent(process.ComponentName, nil)

	_, err := context.ContextComponent(ctx.Context)

	c.Check(err, gc.ErrorMatches, `component "process" disabled`)
	s.Stub.CheckCallNames(c, "Component")
}

func (s *contextSuite) TestGetOkay(c *gc.C) {
	var info, expected, extra process.Info
	expected.Name = "A"

	ctx := context.NewContext(&expected, &extra)
	err := ctx.Get("A", &info)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(info, jc.DeepEquals, expected)
}

func (s *contextSuite) TestGetWrongType(c *gc.C) {
	ctx := context.NewContext()
	err := ctx.Get("A", nil)

	c.Check(err, gc.ErrorMatches, "invalid type: expected process.Info, got .*")
}

func (s *contextSuite) TestGetNotFound(c *gc.C) {
	var info process.Info
	ctx := context.NewContext()
	err := ctx.Get("A", &info)

	c.Check(err, jc.Satisfies, errors.IsNotFound)
}

func (s *contextSuite) TestSetOkay(c *gc.C) {
	var info process.Info
	info.Name = "A"
	ctx := context.NewContext()
	before := ctx.Processes()
	err := ctx.Set("A", &info)
	c.Assert(err, jc.ErrorIsNil)
	after := ctx.Processes()

	c.Check(before, gc.HasLen, 0)
	c.Check(after, jc.DeepEquals, []*process.Info{&info})
}

func (s *contextSuite) TestSetOverwrite(c *gc.C) {
	var info, other process.Info
	info.Name = "A"
	other.Status = process.StatusFailed
	other.Name = "A"
	other.Status = process.StatusPending
	ctx := context.NewContext(&other)
	before := ctx.Processes()
	err := ctx.Set("A", &info)
	c.Assert(err, jc.ErrorIsNil)
	after := ctx.Processes()

	c.Check(before, jc.DeepEquals, []*process.Info{&other})
	c.Check(after, jc.DeepEquals, []*process.Info{&info})
}

func (s *contextSuite) TestSetWrongType(c *gc.C) {
	ctx := context.NewContext()
	before := ctx.Processes()
	err := ctx.Set("A", nil)
	after := ctx.Processes()

	c.Check(err, gc.ErrorMatches, "invalid type: expected process.Info, got .*")
	c.Check(before, gc.HasLen, 0)
	c.Check(after, gc.HasLen, 0)
}

func (s *contextSuite) TestSetNameMismatch(c *gc.C) {
	var info, other process.Info
	info.Name = "B"
	other.Name = "A"
	ctx := context.NewContext(&other)
	before := ctx.Processes()
	err := ctx.Set("A", &info)
	after := ctx.Processes()

	c.Check(err, gc.ErrorMatches, "mismatch on name: A != B")
	c.Check(before, jc.DeepEquals, []*process.Info{&other})
	c.Check(after, jc.DeepEquals, []*process.Info{&other})
}

func (s *contextSuite) TestFlushDirty(c *gc.C) {
	ctx := context.NewContext()
	ctx.Set("A", nil)

	err := ctx.Flush()
	c.Assert(err, gc.ErrorMatches, "not finished")
}

func (s *contextSuite) TestFlushNotDirty(c *gc.C) {
	var info process.Info
	info.Name = "flush-not-dirty"
	ctx := context.NewContext(&info)

	err := ctx.Flush()
	c.Assert(err, jc.ErrorIsNil)

	s.Stub.CheckCallNames(c)
}

func (s *contextSuite) TestFlushEmpty(c *gc.C) {
	ctx := context.NewContext()
	err := ctx.Flush()
	c.Assert(err, jc.ErrorIsNil)

	s.Stub.CheckCallNames(c)
}
