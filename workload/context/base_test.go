// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package context_test

import (
	"reflect"

	"github.com/juju/errors"
	"github.com/juju/testing"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/charm.v5"

	components "github.com/juju/juju/component/all"
	"github.com/juju/juju/utils"
	jujuctesting "github.com/juju/juju/worker/uniter/runner/jujuc/testing"
	"github.com/juju/juju/workload"
	"github.com/juju/juju/workload/context"
)

func init() {
	utils.Must(components.RegisterForServer())
}

type baseSuite struct {
	jujuctesting.ContextSuite
	workload    workload.Info
	definitions map[string]charm.PayloadClass
}

func (s *baseSuite) SetUpTest(c *gc.C) {
	s.ContextSuite.SetUpTest(c)

	s.workload = s.newWorkload("workload A", "docker", "", "")
}

func (s *baseSuite) newWorkload(name, ptype, id, status string) workload.Info {
	info := workload.Info{
		PayloadClass: charm.PayloadClass{
			Name: name,
			Type: ptype,
		},
		Details: workload.Details{
			ID: id,
			Status: workload.PluginStatus{
				State: status,
			},
		},
	}
	if status != "" {
		info.Status = workload.Status{
			State: workload.StateRunning,
		}
	}
	return info
}

func (s *baseSuite) NewHookContext() (*stubHookContext, *jujuctesting.ContextInfo) {
	ctx, info := s.ContextSuite.NewHookContext()
	return &stubHookContext{ctx}, info
}

func checkWorkloads(c *gc.C, workloads, expected []workload.Info) {
	if !c.Check(workloads, gc.HasLen, len(expected)) {
		return
	}
	for _, wl := range workloads {
		matched := false
		for _, expWorkload := range expected {
			if reflect.DeepEqual(wl, expWorkload) {
				matched = true
				break
			}
		}
		if !matched {
			c.Errorf("%#v != %#v", workloads, expected)
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

var _ context.Component = (*stubContextComponent)(nil)

type stubContextComponent struct {
	stub        *testing.Stub
	workloads   map[string]workload.Info
	definitions map[string]charm.PayloadClass
	untracks    map[string]struct{}
}

func newStubContextComponent(stub *testing.Stub) *stubContextComponent {
	return &stubContextComponent{
		stub:        stub,
		workloads:   make(map[string]workload.Info),
		definitions: make(map[string]charm.PayloadClass),
		untracks:    make(map[string]struct{}),
	}
}

func (c *stubContextComponent) Get(id string) (*workload.Info, error) {
	c.stub.AddCall("Get", id)
	if err := c.stub.NextErr(); err != nil {
		return nil, errors.Trace(err)
	}

	info, ok := c.workloads[id]
	if !ok {
		return nil, errors.NotFoundf(id)
	}
	return &info, nil
}

func (c *stubContextComponent) List() ([]string, error) {
	c.stub.AddCall("List")
	if err := c.stub.NextErr(); err != nil {
		return nil, errors.Trace(err)
	}

	var ids []string
	for k := range c.workloads {
		ids = append(ids, k)
	}
	return ids, nil
}

func (c *stubContextComponent) Track(info workload.Info) error {
	c.stub.AddCall("Track", info)
	if err := c.stub.NextErr(); err != nil {
		return errors.Trace(err)
	}

	c.workloads[info.ID()] = info
	return nil
}

func (c *stubContextComponent) Untrack(id string) error {
	c.stub.AddCall("Untrack", id)

	if err := c.stub.NextErr(); err != nil {
		return errors.Trace(err)
	}

	c.untracks[id] = struct{}{}
	return nil
}

func (c *stubContextComponent) SetStatus(class, id, status string) error {
	c.stub.AddCall("SetStatus", class, id, status)
	if err := c.stub.NextErr(); err != nil {
		return errors.Trace(err)
	}

	ID := class + "/" + id
	workload := c.workloads[ID]
	workload.Status.State = status
	workload.Details.Status.State = status
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
	stub        *testing.Stub
	workloads   map[string]workload.Info
	definitions map[string]charm.PayloadClass
}

func newStubAPIClient(stub *testing.Stub) *stubAPIClient {
	return &stubAPIClient{
		stub:        stub,
		workloads:   make(map[string]workload.Info),
		definitions: make(map[string]charm.PayloadClass),
	}
}

func (c *stubAPIClient) setNew(ids ...string) []workload.Info {
	var workloads []workload.Info
	for _, id := range ids {
		name, pluginID := workload.ParseID(id)
		if name == "" {
			panic("missing name")
		}
		if pluginID == "" {
			panic("missing id")
		}
		wl := workload.Info{
			PayloadClass: charm.PayloadClass{
				Name: name,
				Type: "myplugin",
			},
			Status: workload.Status{
				State: workload.StateRunning,
			},
			Details: workload.Details{
				ID: pluginID,
				Status: workload.PluginStatus{
					State: "okay",
				},
			},
		}
		c.workloads[id] = wl
		workloads = append(workloads, wl)
	}
	return workloads
}

func (c *stubAPIClient) List(ids ...string) ([]workload.Info, error) {
	c.stub.AddCall("List", ids)
	if err := c.stub.NextErr(); err != nil {
		return nil, errors.Trace(err)
	}

	var workloads []workload.Info
	if ids == nil {
		for _, wl := range c.workloads {
			workloads = append(workloads, wl)
		}
	} else {
		for _, id := range ids {
			wl, ok := c.workloads[id]
			if !ok {
				return nil, errors.NotFoundf("wl %q", id)
			}
			workloads = append(workloads, wl)
		}
	}
	return workloads, nil
}

func (c *stubAPIClient) Track(workloads ...workload.Info) ([]string, error) {
	c.stub.AddCall("Track", workloads)
	if err := c.stub.NextErr(); err != nil {
		return nil, errors.Trace(err)
	}

	var ids []string
	for _, wl := range workloads {
		c.workloads[wl.Name] = wl
		ids = append(ids, wl.ID())
	}
	return ids, nil
}

func (c *stubAPIClient) Untrack(ids []string) ([]workload.Result, error) {
	c.stub.AddCall("Untrack", ids)
	if err := c.stub.NextErr(); err != nil {
		return nil, errors.Trace(err)
	}

	errs := []workload.Result{}
	for _, id := range ids {
		delete(c.workloads, id)
		errs = append(errs, workload.Result{ID: id})
	}
	return errs, nil
}

func (c *stubAPIClient) SetStatus(class, status string, ids ...string) ([]workload.Result, error) {
	c.stub.AddCall("SetStatus", class, status, ids)
	if err := c.stub.NextErr(); err != nil {
		return nil, errors.Trace(err)
	}

	errs := []workload.Result{}
	for _, id := range ids {
		ID := class + "/" + id
		wkl := c.workloads[ID]
		wkl.Status.State = status
		wkl.Details.Status.State = status
		errs = append(errs, workload.Result{ID: ID})
	}

	return errs, nil
}
