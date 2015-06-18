// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package context

import (
	"github.com/juju/errors"

	"github.com/juju/juju/process"
	"github.com/juju/juju/worker/uniter/runner/jujuc"
)

// APIClient represents the API needs of a Context.
type APIClient interface {
	// List requests the list of registered process IDs from state.
	List() ([]string, error)
	// Get requests the process info for the given ID.
	Get(ids ...string) ([]*process.Info, error)
	// Set sends a request to update state with the provided processes.
	Set(procs ...*process.Info) error
}

// omponent provides the hook context data specific to workload processes.
type Component interface {
	// Get returns the process info corresponding to the given ID.
	Get(procName string) (*process.Info, error)
	// Set records the process info in the hook context.
	Set(procName string, info *process.Info) error
}

// Context is the workload process portion of the hook context.
type Context struct {
	api       APIClient
	processes map[string]*process.Info
	updates   map[string]*process.Info
}

// NewContext returns a new jujuc.ContextComponent for workload processes.
func NewContext(api APIClient, procs ...*process.Info) *Context {
	processes := make(map[string]*process.Info)
	for _, proc := range procs {
		processes[proc.Name] = proc
	}
	return &Context{
		processes: processes,
		api:       api,
	}
}

// NewContextAPI returns a new jujuc.ContextComponent for workload processes.
func NewContextAPI(api APIClient) (*Context, error) {
	ids, err := api.List()
	if err != nil {
		return nil, errors.Trace(err)
	}

	ctx := NewContext(api)
	for _, id := range ids {
		ctx.processes[id] = nil
	}
	return ctx, nil
}

// HookContext is the portion of jujuc.Context used in this package.
type HookContext interface {
	// Component implements jujuc.Context.
	Component(string) (jujuc.ContextComponent, error)
}

// ContextComponent returns the hook context for the workload
// process component.
func ContextComponent(ctx HookContext) (Component, error) {
	found, err := ctx.Component(process.ComponentName)
	if errors.IsNotFound(err) {
		return nil, errors.Errorf("component %q not registered", process.ComponentName)
	}
	if found == nil {
		return nil, errors.Errorf("component %q disabled", process.ComponentName)
	}
	compCtx, ok := found.(Component)
	if !ok {
		return nil, errors.Errorf("wrong component context type registered: %T", found)
	}
	return compCtx, nil
}

func (c *Context) addProc(id string, original *process.Info) error {
	var proc *process.Info
	if original != nil {
		info := *original
		info.Name = id
		proc = &info
	}
	if _, ok := c.processes[id]; !ok {
		c.processes[id] = proc
	} else {
		if proc == nil {
			return errors.Errorf("update can't be nil")
		}
		c.set(id, proc)
	}
	return nil
}

// Processes returns the processes known to the context.
func (c *Context) Processes() ([]*process.Info, error) {
	var procs []*process.Info
	for id, info := range mergeProcMaps(c.processes, c.updates) {
		if info == nil {
			fetched, err := c.api.Get(id)
			if err != nil {
				return nil, errors.Trace(err)
			}
			info = fetched[0]
			c.processes[id] = info
		}
		procs = append(procs, info)
	}
	return procs, nil
}

func mergeProcMaps(procs, updates map[string]*process.Info) map[string]*process.Info {
	// At this point procs and updates have already been checked for
	// nil values so we won't see any here.
	result := make(map[string]*process.Info)
	for k, v := range procs {
		result[k] = v
	}
	for k, v := range updates {
		result[k] = v
	}
	return result
}

// Get returns the process info corresponding to the given ID.
func (c *Context) Get(procName string) (*process.Info, error) {
	actual, ok := c.updates[procName]
	if !ok {
		actual, ok = c.processes[procName]
		if !ok {
			return nil, errors.NotFoundf("%s", procName)
		}
	}
	if actual == nil {
		fetched, err := c.api.Get(procName)
		if err != nil {
			return nil, errors.Trace(err)
		}
		actual = fetched[0]
		c.processes[procName] = actual
	}
	return actual, nil
}

// Set records the process info in the hook context.
func (c *Context) Set(procName string, info *process.Info) error {
	if procName != info.Name {
		return errors.Errorf("mismatch on name: %s != %s", procName, info.Name)
	}
	// TODO(ericsnow) We are likely missing mechanisim for local persistence.

	c.set(procName, info)
	return nil
}

func (c *Context) set(id string, pInfo *process.Info) {
	if c.updates == nil {
		c.updates = make(map[string]*process.Info)
	}
	var info process.Info
	info = *pInfo
	c.updates[id] = &info
}

// Flush implements jujuc.ContextComponent. In this case that means all
// added and updated process.Info in the hook context are pushed to
// Juju state via the API.
func (c *Context) Flush() error {
	if len(c.updates) == 0 {
		return nil
	}

	var updates []*process.Info
	for _, info := range c.updates {
		updates = append(updates, info)
	}
	if err := c.api.Set(updates...); err != nil {
		return errors.Trace(err)
	}

	for k, v := range c.updates {
		c.processes[k] = v
	}
	c.updates = nil
	return nil
}
