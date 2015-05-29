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
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/worker/uniter/runner/jujuc"
)

type contextSuite struct {
	coretesting.BaseSuite
	stub    *testing.Stub
	ctx     *fakeContext
	compCtx *fakeContextComponent
}

var _ = gc.Suite(&contextSuite{})

func (s *contextSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)

	s.stub = &testing.Stub{}
	s.ctx = &fakeContext{
		Stub:       s.stub,
		components: make(map[string]jujuc.ContextComponent),
	}
	s.compCtx = &fakeContextComponent{
		Stub: s.stub,
	}
}

func (s *contextSuite) TestNewContextEmpty(c *gc.C) {
	ctx := context.NewContext()
	procs := ctx.Processes()

	c.Check(procs, gc.HasLen, 0)
}

func (s *contextSuite) TestNewContextPrePopulated(c *gc.C) {
	expected := []process.Info{{}, {}}
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
	expected := context.NewContext()
	s.ctx.components[process.ComponentName] = expected

	compCtx, err := context.ContextComponent(s.ctx)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(compCtx, gc.Equals, expected)
	s.stub.CheckCallNames(c, "Component")
	s.stub.CheckCall(c, 0, "Component", "process")
}

func (s *contextSuite) TestContextComponentMissing(c *gc.C) {
	_, err := context.ContextComponent(s.ctx)

	c.Check(err, gc.ErrorMatches, `component "process" not registered`)
	s.stub.CheckCallNames(c, "Component")
}

func (s *contextSuite) TestContextComponentWrong(c *gc.C) {
	s.ctx.components[process.ComponentName] = s.compCtx

	_, err := context.ContextComponent(s.ctx)

	c.Check(err, gc.ErrorMatches, "wrong component context type registered: .*")
	s.stub.CheckCallNames(c, "Component")
}

func (s *contextSuite) TestContextComponentDisabled(c *gc.C) {
	s.ctx.components[process.ComponentName] = nil

	_, err := context.ContextComponent(s.ctx)

	c.Check(err, gc.ErrorMatches, `component "process" disabled`)
	s.stub.CheckCallNames(c, "Component")
}

func (s *contextSuite) TestGetOkay(c *gc.C) {
	var info, expected, extra process.Info
	expected.Name = "A"

	ctx := context.NewContext(expected, extra)
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
	c.Check(after, jc.DeepEquals, []process.Info{info})
}

func (s *contextSuite) TestSetOverwrite(c *gc.C) {
	var info, other process.Info
	info.Name = "A"
	other.Status = process.StatusFailed
	other.Name = "A"
	other.Status = process.StatusPending
	ctx := context.NewContext(other)
	before := ctx.Processes()
	err := ctx.Set("A", &info)
	c.Assert(err, jc.ErrorIsNil)
	after := ctx.Processes()

	c.Check(before, jc.DeepEquals, []process.Info{other})
	c.Check(after, jc.DeepEquals, []process.Info{info})
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
	ctx := context.NewContext(other)
	before := ctx.Processes()
	err := ctx.Set("A", &info)
	after := ctx.Processes()

	c.Check(err, gc.ErrorMatches, "mismatch on name: A != B")
	c.Check(before, jc.DeepEquals, []process.Info{other})
	c.Check(after, jc.DeepEquals, []process.Info{other})
}

func (s *contextSuite) TestFlush(c *gc.C) {
	ctx := context.NewContext()
	err := ctx.Flush()
	c.Assert(err, jc.ErrorIsNil)

	// TODO(ericsnow) Check calls here.
}

type fakeContextComponent struct {
	*testing.Stub
}

func (fcc fakeContextComponent) Get(name string, result interface{}) error {
	fcc.AddCall("Get", name, result)
	return fcc.NextErr()
}

func (fcc fakeContextComponent) Set(name string, value interface{}) error {
	fcc.AddCall("Set", name, value)
	return fcc.NextErr()
}

func (fcc fakeContextComponent) Flush() error {
	fcc.AddCall("Flush")
	return fcc.NextErr()
}

type fakeContext struct {
	*testing.Stub
	components map[string]jujuc.ContextComponent
}

func (fc fakeContext) Component(name string) (jujuc.ContextComponent, bool) {
	fc.AddCall("Component", name)
	fc.NextErr()

	compCtx, ok := fc.components[name]
	return compCtx, ok
}
