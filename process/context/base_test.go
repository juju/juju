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
	proc process.Info
}

func (s *baseSuite) SetUpTest(c *gc.C) {
	s.ContextSuite.SetUpTest(c)

	s.proc = s.newProc("proc A", "docker", "", "")
}

func (s *baseSuite) newProc(name, ptype, id, status string) process.Info {
	info := process.Info{
		Process: charm.Process{
			Name: name,
			Type: ptype,
		},
		Details: process.Details{
			ID: id,
			Status: process.PluginStatus{
				State: status,
			},
		},
	}
	if status != "" {
		info.Status = process.Status{
			State: process.StateRunning,
		}
	}
	return info
}

func (s *baseSuite) NewHookContext() (*stubHookContext, *jujuctesting.ContextInfo) {
	ctx, info := s.ContextSuite.NewHookContext()
	return &stubHookContext{ctx}, info
}

func checkProcs(c *gc.C, procs, expected []process.Info) {
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

var _ context.Component = (*stubContextComponent)(nil)

type stubContextComponent struct {
	stub        *testing.Stub
	procs       map[string]process.Info
	definitions map[string]charm.Process
	untracks    map[string]struct{}
	plugin      process.Plugin
}

func newStubContextComponent(stub *testing.Stub) *stubContextComponent {
	return &stubContextComponent{
		stub:        stub,
		procs:       make(map[string]process.Info),
		definitions: make(map[string]charm.Process),
		untracks:    make(map[string]struct{}),
	}
}

func (c *stubContextComponent) Plugin(info *process.Info) (process.Plugin, error) {
	c.stub.AddCall("Plugin", info)
	if err := c.stub.NextErr(); err != nil {
		return nil, errors.Trace(err)
	}

	if c.plugin == nil {
		return &stubPlugin{stub: c.stub}, nil
	}
	return c.plugin, nil
}

func (c *stubContextComponent) Get(id string) (*process.Info, error) {
	c.stub.AddCall("Get", id)
	if err := c.stub.NextErr(); err != nil {
		return nil, errors.Trace(err)
	}

	info, ok := c.procs[id]
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
	for k := range c.procs {
		ids = append(ids, k)
	}
	return ids, nil
}

func (c *stubContextComponent) Set(info process.Info) error {
	c.stub.AddCall("Set", info)
	if err := c.stub.NextErr(); err != nil {
		return errors.Trace(err)
	}

	c.procs[info.ID()] = info
	return nil
}

func (c *stubContextComponent) Untrack(id string) {
	c.stub.AddCall("Untrack", id)

	c.untracks[id] = struct{}{}
}

func (c *stubContextComponent) ListDefinitions() ([]charm.Process, error) {
	c.stub.AddCall("ListDefinitions")
	if err := c.stub.NextErr(); err != nil {
		return nil, errors.Trace(err)
	}

	var definitions []charm.Process
	for _, definition := range c.definitions {
		definitions = append(definitions, definition)
	}
	return definitions, nil
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
	procs       map[string]process.Info
	definitions map[string]charm.Process
}

func newStubAPIClient(stub *testing.Stub) *stubAPIClient {
	return &stubAPIClient{
		stub:        stub,
		procs:       make(map[string]process.Info),
		definitions: make(map[string]charm.Process),
	}
}

func (c *stubAPIClient) setNew(ids ...string) []process.Info {
	var procs []process.Info
	for _, id := range ids {
		name, pluginID := process.ParseID(id)
		if name == "" {
			panic("missing name")
		}
		if pluginID == "" {
			panic("missing id")
		}
		proc := process.Info{
			Process: charm.Process{
				Name: name,
				Type: "myplugin",
			},
			Status: process.Status{
				State: process.StateRunning,
			},
			Details: process.Details{
				ID: pluginID,
				Status: process.PluginStatus{
					State: "okay",
				},
			},
		}
		c.procs[id] = proc
		procs = append(procs, proc)
	}
	return procs
}

func (c *stubAPIClient) AllDefinitions() ([]charm.Process, error) {
	c.stub.AddCall("AllDefinitions")
	if err := c.stub.NextErr(); err != nil {
		return nil, errors.Trace(err)
	}

	var definitions []charm.Process
	for _, definition := range c.definitions {
		definitions = append(definitions, definition)
	}
	return definitions, nil
}

func (c *stubAPIClient) ListProcesses(ids ...string) ([]process.Info, error) {
	c.stub.AddCall("ListProcesses", ids)
	if err := c.stub.NextErr(); err != nil {
		return nil, errors.Trace(err)
	}

	var procs []process.Info
	if ids == nil {
		for _, proc := range c.procs {
			procs = append(procs, proc)
		}
	} else {
		for _, id := range ids {
			proc, ok := c.procs[id]
			if !ok {
				return nil, errors.NotFoundf("proc %q", id)
			}
			procs = append(procs, proc)
		}
	}
	return procs, nil
}

func (c *stubAPIClient) RegisterProcesses(procs ...process.Info) ([]string, error) {
	c.stub.AddCall("RegisterProcesses", procs)
	if err := c.stub.NextErr(); err != nil {
		return nil, errors.Trace(err)
	}

	var ids []string
	for _, proc := range procs {
		c.procs[proc.Name] = proc
		ids = append(ids, proc.ID())
	}
	return ids, nil
}

func (c *stubAPIClient) Untrack(ids []string) error {
	c.stub.AddCall("Untrack", ids)
	if err := c.stub.NextErr(); err != nil {
		return errors.Trace(err)
	}

	for _, id := range ids {
		delete(c.procs, id)
	}
	return nil
}

func (c *stubAPIClient) SetProcessesStatus(status process.Status, pluginStatus process.PluginStatus, ids ...string) error {
	c.stub.AddCall("SetProcessesStatus", status, pluginStatus, ids)
	if err := c.stub.NextErr(); err != nil {
		return errors.Trace(err)
	}

	for _, id := range ids {
		proc := c.procs[id]
		proc.Status = status
		proc.Details.Status = pluginStatus
	}
	return nil
}

type stubPlugin struct {
	stub    *testing.Stub
	details process.Details
	status  process.PluginStatus
}

func (c *stubPlugin) Launch(definition charm.Process) (process.Details, error) {
	c.stub.AddCall("Launch", definition)
	if err := c.stub.NextErr(); err != nil {
		return c.details, errors.Trace(err)
	}

	return c.details, nil
}

func (c *stubPlugin) Destroy(id string) error {
	c.stub.AddCall("Destroy", id)
	if err := c.stub.NextErr(); err != nil {
		return errors.Trace(err)
	}

	return nil
}

func (c *stubPlugin) Status(id string) (process.PluginStatus, error) {
	c.stub.AddCall("Status", id)
	if err := c.stub.NextErr(); err != nil {
		return c.status, errors.Trace(err)
	}

	return c.status, nil
}
