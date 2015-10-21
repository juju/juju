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

func (c *stubContextComponent) Get(class, id string) (*workload.Info, error) {
	c.stub.AddCall("Get", class, id)
	if err := c.stub.NextErr(); err != nil {
		return nil, errors.Trace(err)
	}

	fullID := workload.BuildID(class, id)
	info, ok := c.workloads[fullID]
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

	var fullIDs []string
	for k := range c.workloads {
		fullIDs = append(fullIDs, k)
	}
	return fullIDs, nil
}

func (c *stubContextComponent) Track(info workload.Info) error {
	c.stub.AddCall("Track", info)
	if err := c.stub.NextErr(); err != nil {
		return errors.Trace(err)
	}

	c.workloads[info.ID()] = info
	return nil
}

func (c *stubContextComponent) Untrack(class, id string) error {
	c.stub.AddCall("Untrack", class, id)

	if err := c.stub.NextErr(); err != nil {
		return errors.Trace(err)
	}

	fullID := workload.BuildID(class, id)
	c.untracks[fullID] = struct{}{}
	return nil
}

func (c *stubContextComponent) SetStatus(class, id, status string) error {
	c.stub.AddCall("SetStatus", class, id, status)
	if err := c.stub.NextErr(); err != nil {
		return errors.Trace(err)
	}

	fullID := workload.BuildID(class, id)
	workload := c.workloads[fullID]
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
	stub *testing.Stub
	// TODO(ericsnow) Use id for the key rather than Info.ID().
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

func (c *stubAPIClient) setNew(fullIDs ...string) []workload.Info {
	var workloads []workload.Info
	for _, id := range fullIDs {
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

func (c *stubAPIClient) List(fullIDs ...string) ([]workload.Result, error) {
	c.stub.AddCall("List", fullIDs)
	if err := c.stub.NextErr(); err != nil {
		return nil, errors.Trace(err)
	}

	var results []workload.Result
	if fullIDs == nil {
		for id, wl := range c.workloads {
			results = append(results, workload.Result{
				ID:       id,
				Workload: &wl,
			})
		}
	} else {
		for _, id := range fullIDs {
			wl, ok := c.workloads[id]
			if !ok {
				return nil, errors.NotFoundf("wl %q", id)
			}
			results = append(results, workload.Result{
				ID:       id,
				Workload: &wl,
			})
		}
	}
	return results, nil
}

func (c *stubAPIClient) Track(workloads ...workload.Info) ([]workload.Result, error) {
	c.stub.AddCall("Track", workloads)
	if err := c.stub.NextErr(); err != nil {
		return nil, errors.Trace(err)
	}

	var results []workload.Result
	for _, wl := range workloads {
		id := wl.ID()
		c.workloads[id] = wl
		results = append(results, workload.Result{
			ID:       id,
			Workload: &wl,
		})
	}
	return results, nil
}

func (c *stubAPIClient) Untrack(fullIDs ...string) ([]workload.Result, error) {
	c.stub.AddCall("Untrack", fullIDs)
	if err := c.stub.NextErr(); err != nil {
		return nil, errors.Trace(err)
	}

	errs := []workload.Result{}
	for _, id := range fullIDs {
		delete(c.workloads, id)
		errs = append(errs, workload.Result{ID: id})
	}
	return errs, nil
}

func (c *stubAPIClient) SetStatus(status string, fullIDs ...string) ([]workload.Result, error) {
	c.stub.AddCall("SetStatus", status, fullIDs)
	if err := c.stub.NextErr(); err != nil {
		return nil, errors.Trace(err)
	}

	errs := []workload.Result{}
	for _, id := range fullIDs {
		wkl := c.workloads[id]
		wkl.Status.State = status
		wkl.Details.Status.State = status
		errs = append(errs, workload.Result{ID: id})
	}

	return errs, nil
}
