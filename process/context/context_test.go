// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package context_test

import (
	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/process"
	"github.com/juju/juju/process/context"
	jujuctesting "github.com/juju/juju/worker/uniter/runner/jujuc/testing"
)

type baseSuite struct {
	jujuctesting.ContextSuite
	proc      *process.Info
	compCtx   *context.Context
	apiClient context.APIClient
}

func (s *baseSuite) SetUpTest(c *gc.C) {
	s.ContextSuite.SetUpTest(c)

	s.apiClient = &stubAPIClient{stub: s.Stub}
	proc := process.NewInfo("proc A", "docker")
	compCtx := context.NewContext(s.apiClient, proc)

	s.Ctx = s.HookContext("u/0", nil)
	s.Ctx.SetComponent(process.ComponentName, compCtx)

	s.proc = proc
	s.compCtx = compCtx
}

type contextSuite struct {
	baseSuite
}

var _ = gc.Suite(&contextSuite{})

func (s *contextSuite) TestNewContextEmpty(c *gc.C) {
	ctx := context.NewContext(s.apiClient)
	procs, err := ctx.Processes()
	c.Assert(err, jc.ErrorIsNil)

	c.Check(procs, gc.HasLen, 0)
}

func (s *contextSuite) TestNewContextPrePopulated(c *gc.C) {
	expected := []*process.Info{{}, {}}
	expected[0].Name = "A"
	expected[1].Name = "B"

	ctx := context.NewContext(s.apiClient, expected...)
	procs, err := ctx.Processes()
	c.Assert(err, jc.ErrorIsNil)

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
	expected := context.NewContext(s.apiClient)
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

	ctx := context.NewContext(s.apiClient, &expected, &extra)
	err := ctx.Get("A", &info)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(info, jc.DeepEquals, expected)
}

func (s *contextSuite) TestGetWrongType(c *gc.C) {
	ctx := context.NewContext(s.apiClient)
	err := ctx.Get("A", nil)

	c.Check(err, gc.ErrorMatches, "invalid type: expected process.Info, got .*")
}

func (s *contextSuite) TestGetNotFound(c *gc.C) {
	var info process.Info
	ctx := context.NewContext(s.apiClient)
	err := ctx.Get("A", &info)

	c.Check(err, jc.Satisfies, errors.IsNotFound)
}

func (s *contextSuite) TestSetOkay(c *gc.C) {
	var info process.Info
	info.Name = "A"
	ctx := context.NewContext(s.apiClient)
	before, err := ctx.Processes()
	c.Assert(err, jc.ErrorIsNil)
	err = ctx.Set("A", &info)
	c.Assert(err, jc.ErrorIsNil)
	after, err := ctx.Processes()
	c.Assert(err, jc.ErrorIsNil)

	c.Check(before, gc.HasLen, 0)
	c.Check(after, jc.DeepEquals, []*process.Info{&info})
}

func (s *contextSuite) TestSetOverwrite(c *gc.C) {
	var info, other process.Info
	info.Name = "A"
	other.Status = process.StatusFailed
	other.Name = "A"
	other.Status = process.StatusPending
	ctx := context.NewContext(s.apiClient, &other)
	before, err := ctx.Processes()
	c.Assert(err, jc.ErrorIsNil)
	err = ctx.Set("A", &info)
	c.Assert(err, jc.ErrorIsNil)
	after, err := ctx.Processes()
	c.Assert(err, jc.ErrorIsNil)

	c.Check(before, jc.DeepEquals, []*process.Info{&other})
	c.Check(after, jc.DeepEquals, []*process.Info{&info})
}

func (s *contextSuite) TestSetWrongType(c *gc.C) {
	ctx := context.NewContext(s.apiClient)
	before, err2 := ctx.Processes()
	c.Assert(err2, jc.ErrorIsNil)
	err := ctx.Set("A", &struct{}{})
	after, err2 := ctx.Processes()
	c.Assert(err2, jc.ErrorIsNil)

	c.Check(err, gc.ErrorMatches, "invalid type: expected process.Info, got .*")
	c.Check(before, gc.HasLen, 0)
	c.Check(after, gc.HasLen, 0)
}

func (s *contextSuite) TestSetNameMismatch(c *gc.C) {
	var info, other process.Info
	info.Name = "B"
	other.Name = "A"
	ctx := context.NewContext(s.apiClient, &other)
	before, err2 := ctx.Processes()
	c.Assert(err2, jc.ErrorIsNil)
	err := ctx.Set("A", &info)
	after, err2 := ctx.Processes()
	c.Assert(err2, jc.ErrorIsNil)

	c.Check(err, gc.ErrorMatches, "mismatch on name: A != B")
	c.Check(before, jc.DeepEquals, []*process.Info{&other})
	c.Check(after, jc.DeepEquals, []*process.Info{&other})
}

func (s *contextSuite) TestFlushDirty(c *gc.C) {
	var info process.Info
	info.Name = "A"

	ctx := context.NewContext(s.apiClient)
	err := ctx.Set("A", &info)
	c.Assert(err, jc.ErrorIsNil)

	err = ctx.Flush()
	c.Assert(err, jc.ErrorIsNil)

	s.Stub.CheckCallNames(c, "Set")
}

func (s *contextSuite) TestFlushNotDirty(c *gc.C) {
	var info process.Info
	info.Name = "flush-not-dirty"
	ctx := context.NewContext(s.apiClient, &info)

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

type stubAPIClient struct {
	stub  *testing.Stub
	procs map[string]*process.Info
}

func (c *stubAPIClient) List() ([]string, error) {
	c.stub.AddCall("List")
	if err := c.stub.NextErr(); err != nil {
		return nil, errors.Trace(err)
	}

	var ids []string
	for k := range c.procs {
		ids = append(ids, k)
	}
	return ids, nil
}

func (c *stubAPIClient) Get(ids ...string) ([]*process.Info, error) {
	c.stub.AddCall("Get", ids)
	if err := c.stub.NextErr(); err != nil {
		return nil, errors.Trace(err)
	}

	var procs []*process.Info
	for _, id := range ids {
		proc, ok := c.procs[id]
		if !ok {
			return nil, errors.NotFoundf("proc %q", id)
		}
		procs = append(procs, proc)
	}
	return procs, nil
}

func (c *stubAPIClient) Set(procs ...*process.Info) error {
	c.stub.AddCall("Set", procs)
	if err := c.stub.NextErr(); err != nil {
		return errors.Trace(err)
	}

	if c.procs == nil {
		c.procs = make(map[string]*process.Info)
	}
	for _, proc := range procs {
		c.procs[proc.Name] = proc
	}
	return nil
}
