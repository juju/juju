// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package context_test

import (
	"reflect"

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
	apiClient *stubAPIClient
	Ctx       *jujuctesting.Context
}

func (s *baseSuite) SetUpTest(c *gc.C) {
	s.ContextSuite.SetUpTest(c)

	s.apiClient = newStubAPIClient(s.Stub)
	proc := process.NewInfoUnvalidated("proc A", "docker")
	compCtx := context.NewContext(s.apiClient, proc)

	hctx, info := s.NewHookContext()
	info.SetComponent(process.ComponentName, compCtx)

	s.proc = proc
	s.compCtx = compCtx
	s.Ctx = hctx
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

func (s *contextSuite) TestNewContextAPIOkay(c *gc.C) {
	expected := s.apiClient.setNew("A")

	ctx, err := context.NewContextAPI(s.apiClient)
	c.Assert(err, jc.ErrorIsNil)

	procs, err := ctx.Processes()
	c.Assert(err, jc.ErrorIsNil)

	c.Check(procs, jc.DeepEquals, expected)
}

func (s *contextSuite) TestNewContextAPICalls(c *gc.C) {
	s.apiClient.setNew("A")

	_, err := context.NewContextAPI(s.apiClient)
	c.Assert(err, jc.ErrorIsNil)

	s.Stub.CheckCallNames(c, "List")
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
	s.Stub.CheckCallNames(c, "List")
}

func (s *contextSuite) TestContextComponentOkay(c *gc.C) {
	hctx, info := s.NewHookContext()
	expected := context.NewContext(s.apiClient)
	info.SetComponent(process.ComponentName, expected)

	compCtx, err := context.ContextComponent(hctx)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(compCtx, gc.Equals, expected)
	s.Stub.CheckCallNames(c, "Component")
	s.Stub.CheckCall(c, 0, "Component", "process")
}

func (s *contextSuite) TestContextComponentMissing(c *gc.C) {
	hctx, _ := s.NewHookContext()
	_, err := context.ContextComponent(hctx)

	c.Check(err, gc.ErrorMatches, `component "process" not registered`)
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

	c.Check(err, gc.ErrorMatches, `component "process" disabled`)
	s.Stub.CheckCallNames(c, "Component")
}

func (s *contextSuite) TestProcessesOkay(c *gc.C) {
	expected := []*process.Info{{}, {}, {}}
	expected[0].Name = "A"
	expected[1].Name = "B"
	expected[2].Name = "C"

	ctx := context.NewContext(s.apiClient, expected...)
	procs, err := ctx.Processes()
	c.Assert(err, jc.ErrorIsNil)

	checkProcs(c, procs, expected)
	s.Stub.CheckCallNames(c)
}

func (s *contextSuite) TestProcessesAPI(c *gc.C) {
	expected := s.apiClient.setNew("A", "B", "C")

	ctx := context.NewContext(s.apiClient)
	context.AddProc(ctx, "A", s.apiClient.procs["A"])
	context.AddProc(ctx, "B", nil)
	context.AddProc(ctx, "C", nil)

	procs, err := ctx.Processes()
	c.Assert(err, jc.ErrorIsNil)

	checkProcs(c, procs, expected)
	s.Stub.CheckCallNames(c, "Get", "Get")
}

func (s *contextSuite) TestProcessesEmpty(c *gc.C) {
	ctx := context.NewContext(s.apiClient)
	procs, err := ctx.Processes()
	c.Assert(err, jc.ErrorIsNil)

	c.Check(procs, gc.HasLen, 0)
	s.Stub.CheckCallNames(c)
}

func (s *contextSuite) TestProcessesAdditions(c *gc.C) {
	expected := s.apiClient.setNew("A", "B")
	var infoC, infoD process.Info
	infoC.Name = "C"
	infoD.Name = "D"
	expected = append(expected, &infoC, &infoD)

	ctx := context.NewContext(s.apiClient, expected[0])
	context.AddProc(ctx, "B", nil)
	ctx.Set("C", &infoC)
	ctx.Set("D", &infoD)

	procs, err := ctx.Processes()
	c.Assert(err, jc.ErrorIsNil)

	checkProcs(c, procs, expected)
	s.Stub.CheckCallNames(c, "Get")
}

func (s *contextSuite) TestProcessesOverrides(c *gc.C) {
	expected := s.apiClient.setNew("A", "B")
	var infoB, infoC process.Info
	infoB.Name = "B"
	infoC.Name = "C"
	expected = append(expected[:1], &infoB, &infoC)

	ctx := context.NewContext(s.apiClient)
	context.AddProc(ctx, "A", nil)
	context.AddProc(ctx, "B", nil)
	ctx.Set("B", &infoB)
	ctx.Set("C", &infoC)

	procs, err := ctx.Processes()
	c.Assert(err, jc.ErrorIsNil)

	checkProcs(c, procs, expected)
	s.Stub.CheckCallNames(c, "Get")
}

func (s *contextSuite) TestGetOkay(c *gc.C) {
	var expected, extra process.Info
	expected.Name = "A"

	ctx := context.NewContext(s.apiClient, &expected, &extra)
	info, err := ctx.Get("A")
	c.Assert(err, jc.ErrorIsNil)

	c.Check(info, jc.DeepEquals, &expected)
	s.Stub.CheckCallNames(c)
}

func (s *contextSuite) TestGetAPIPull(c *gc.C) {
	procs := s.apiClient.setNew("A", "B")
	expected := procs[0]

	ctx := context.NewContext(s.apiClient, procs[1])
	context.AddProc(ctx, "A", nil)

	info, err := ctx.Get("A")
	c.Assert(err, jc.ErrorIsNil)

	c.Check(info, jc.DeepEquals, expected)
	s.Stub.CheckCallNames(c, "Get")
}

func (s *contextSuite) TestGetAPINoPull(c *gc.C) {
	procs := s.apiClient.setNew("A", "B")
	expected := procs[0]

	ctx := context.NewContext(s.apiClient, procs...)

	info, err := ctx.Get("A")
	c.Assert(err, jc.ErrorIsNil)

	c.Check(info, jc.DeepEquals, expected)
	s.Stub.CheckCallNames(c)
}

func (s *contextSuite) TestGetOverride(c *gc.C) {
	procs := s.apiClient.setNew("A", "B")
	expected := procs[0]

	ctx := context.NewContext(s.apiClient, procs[1])
	context.AddProc(ctx, "A", nil)
	context.AddProc(ctx, "A", expected)

	info, err := ctx.Get("A")
	c.Assert(err, jc.ErrorIsNil)

	c.Check(info, jc.DeepEquals, expected)
	s.Stub.CheckCallNames(c)
}

func (s *contextSuite) TestGetNotFound(c *gc.C) {
	ctx := context.NewContext(s.apiClient)
	_, err := ctx.Get("A")

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

func checkProcs(c *gc.C, procs, expected []*process.Info) {
	if !c.Check(procs, gc.HasLen, len(expected)) {
		return
	}
	for _, proc := range procs {
		matched := false
		for _, expProc := range expected {
			if reflect.DeepEqual(proc, expProc) {
				matched = true
				break
			}
		}
		if !matched {
			c.Errorf("%#v != %#v", procs, expected)
			return
		}
	}
}

type stubContextComponent struct {
	stub  *testing.Stub
	procs map[string]*process.Info
}

func newStubContextComponent(stub *testing.Stub) *stubContextComponent {
	return &stubContextComponent{
		stub:  stub,
		procs: make(map[string]*process.Info),
	}
}

func (c *stubContextComponent) Get(procName string) (*process.Info, error) {
	c.stub.AddCall("Get", procName)
	if err := c.stub.NextErr(); err != nil {
		return nil, errors.Trace(err)
	}

	info, ok := c.procs[procName]
	if !ok {
		return nil, errors.NotFoundf(procName)
	}
	return info, nil
}

func (c *stubContextComponent) Set(procName string, info *process.Info) error {
	c.stub.AddCall("Set", procName, info)
	if err := c.stub.NextErr(); err != nil {
		return errors.Trace(err)
	}

	if info.Name != procName {
		return errors.Errorf("name mismatch")
	}
	c.procs[procName] = info
	return nil
}

func (c *stubContextComponent) Flush() error {
	c.stub.AddCall("Flush")
	if err := c.stub.NextErr(); err != nil {
		return errors.Trace(err)
	}

	return nil
}

type stubAPIClient struct {
	stub  *testing.Stub
	procs map[string]*process.Info
}

func newStubAPIClient(stub *testing.Stub) *stubAPIClient {
	return &stubAPIClient{
		stub:  stub,
		procs: make(map[string]*process.Info),
	}
}

func (c *stubAPIClient) setNew(ids ...string) []*process.Info {
	var procs []*process.Info
	for _, id := range ids {
		var proc process.Info
		proc.Name = id
		c.procs[id] = &proc
		procs = append(procs, &proc)
	}
	return procs
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

	for _, proc := range procs {
		c.procs[proc.Name] = proc
	}
	return nil
}
