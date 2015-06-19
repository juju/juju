// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package context_test

import (
	"reflect"

	"github.com/juju/errors"
	"github.com/juju/testing"
	gc "gopkg.in/check.v1"

	components "github.com/juju/juju/component/all"
	"github.com/juju/juju/process"
	"github.com/juju/juju/process/context"
	"github.com/juju/juju/utils"
	jujuctesting "github.com/juju/juju/worker/uniter/runner/jujuc/testing"
)

func init() {
	utils.Must(components.RegisterForServer())
}

type baseSuite struct {
	jujuctesting.ContextSuite
	proc      *process.Info
	compCtx   *context.Context
	apiClient *stubAPIClient
	Ctx       *stubHookContext
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

func (s *baseSuite) NewHookContext() (*stubHookContext, *jujuctesting.ContextInfo) {
	ctx, info := s.ContextSuite.NewHookContext()
	return &stubHookContext{ctx}, info
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

type stubHookContext struct {
	*jujuctesting.Context
}

func (c stubHookContext) Component(name string) (context.Component, error) {
	found, err := c.Context.Component(name)
	if err != nil {
		return nil, errors.Trace(err)
	}
	compCtx, ok := found.(context.Component)
	if !ok && found != nil {
		return nil, errors.Errorf("wrong component context type registered: %T", found)
	}
	return compCtx, nil
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
