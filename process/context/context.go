// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package context

import (
	"github.com/juju/errors"

	"github.com/juju/juju/process"
	"github.com/juju/juju/process/api"
	"github.com/juju/juju/worker/uniter/runner"
	"github.com/juju/juju/worker/uniter/runner/jujuc"
)

func init() {
	// TODO(ericsnow) Have registration handled by a "third party"?
	runner.RegisterComponentFunc(process.ComponentName,
		// TODO(ericsnow) This should be done in a way (or a place)
		// such that we don't have to import runner or jujuc.
		func() (jujuc.ContextComponent, error) {
			// TODO(ericsnow) The API client or facade should be passed
			// in to the factory func and passed to NewClient.
			client, err := api.NewClient()
			if err != nil {
				return nil, errors.Trace(err)
			}
			component, err := NewContextAPI(client)
			if err != nil {
				return nil, errors.Trace(err)
			}
			return component, nil
		},
	)
}

// APIClient represents the API needs of a Context.
type APIClient interface {
	// List requests the list of registered process IDs from state.
	List() ([]string, error)
	// Get requests the process info for the given ID.
	Get(ids ...string) ([]*process.Info, error)
	// Set sends a request to update state with the provided processes.
	Set(procs ...*process.Info) error
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

// NewContext returns a new jujuc.ContextComponent for workload processes.
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
func ContextComponent(ctx HookContext) (*Context, error) {
	found, err := ctx.Component(process.ComponentName)
	if errors.IsNotFound(err) {
		return nil, errors.Errorf("component %q not registered", process.ComponentName)
	}
	if found == nil {
		return nil, errors.Errorf("component %q disabled", process.ComponentName)
	}
	compCtx, ok := found.(*Context)
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
		c.updates[id] = proc
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
	result := make(map[string]*process.Info)
	for k, v := range procs {
		result[k] = v
	}
	for k, v := range updates {
		if v == nil {
			// This should never happen.
			panic("info in updates unexpectedly nil")
		}
		result[k] = v
	}
	return result
}

// Get implements jujuc.ContextComponent.
func (c *Context) Get(id string, result interface{}) error {
	info, ok := result.(*process.Info)
	if !ok {
		return errors.Errorf("invalid type: expected process.Info, got %T", result)
	}

	actual, ok := c.updates[id]
	if !ok {
		actual, ok = c.processes[id]
		if !ok {
			return errors.NotFoundf("%s", id)
		}
	}
	if actual == nil {
		fetched, err := c.api.Get(id)
		if err != nil {
			return errors.Trace(err)
		}
		actual = fetched[0]
		c.processes[id] = actual
	}
	*info = *actual
	return nil
}

// Set implements jujuc.ContextComponent.
func (c *Context) Set(id string, value interface{}) error {
	pInfo, ok := value.(*process.Info)
	if !ok {
		return errors.Errorf("invalid type: expected process.Info, got %T", value)
	}

	if id != pInfo.Name {
		return errors.Errorf("mismatch on name: %s != %s", id, pInfo.Name)
	}

	if c.updates == nil {
		c.updates = make(map[string]*process.Info)
	}
	var info process.Info
	info = *pInfo
	c.updates[id] = &info
	return nil
}

// Flush implements jujuc.ContextComponent.
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
