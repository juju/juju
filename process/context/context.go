// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package context

import (
	"sort"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"gopkg.in/juju/charm.v5"

	"github.com/juju/juju/process"
)

var logger = loggo.GetLogger("juju.process.context")

// TODO(ericsnow) Normalize method names across all the arch. layers
// (e.g. List->AllProcesses, Get->Processes, Set->AddProcess).

// APIClient represents the API needs of a Context.
type APIClient interface {
	// List requests the list of registered process IDs from state.
	List() ([]string, error)
	// Get requests the process info for the given ID.
	Get(ids ...string) ([]*process.Info, error)
	// Set sends a request to update state with the provided processes.
	Set(procs ...*process.Info) error
	// AllDefinitions returns the process definitions found in the
	// unit's metadata.
	AllDefinitions() ([]charm.Process, error)
}

// TODO(ericsnow) Rename Get and Set to more specifically describe what
// they are for.

// omponent provides the hook context data specific to workload processes.
type Component interface {
	// Get returns the process info corresponding to the given ID.
	Get(id string) (*process.Info, error)
	// Set records the process info in the hook context.
	Set(info process.Info) error
	// List returns the list of registered process IDs.
	List() ([]string, error)
	// ListDefinitions returns the charm-defined processes.
	ListDefinitions() ([]charm.Process, error)
	// Flush pushes the hook context data out to state.
	Flush() error
}

// Context is the workload process portion of the hook context.
type Context struct {
	api       APIClient
	processes map[string]*process.Info
	updates   map[string]*process.Info
}

// NewContext returns a new jujuc.ContextComponent for workload processes.
func NewContext(api APIClient) *Context {
	return &Context{
		api:       api,
		processes: make(map[string]*process.Info),
		updates:   make(map[string]*process.Info),
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
	Component(string) (Component, error)
}

// ContextComponent returns the hook context for the workload
// process component.
func ContextComponent(ctx HookContext) (Component, error) {
	compCtx, err := ctx.Component(process.ComponentName)
	if errors.IsNotFound(err) {
		return nil, errors.Errorf("component %q not registered", process.ComponentName)
	}
	if err != nil {
		return nil, errors.Trace(err)
	}
	if compCtx == nil {
		return nil, errors.Errorf("component %q disabled", process.ComponentName)
	}
	return compCtx, nil
}

// TODO(ericsnow) Should we build in refreshes in all the methods?

// Processes returns the processes known to the context.
func (c *Context) Processes() ([]*process.Info, error) {
	var procs []*process.Info
	for id, info := range mergeProcMaps(c.processes, c.updates) {
		if info == nil {
			fetched, err := c.fetch(id)
			if err != nil {
				return nil, errors.Trace(err)
			}
			info = fetched
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

func (c *Context) fetch(id string) (*process.Info, error) {
	fetched, err := c.api.Get(id)
	if err != nil {
		return nil, errors.Trace(err)
	}
	proc := fetched[0]
	c.processes[id] = proc
	return proc, nil
}

// Get returns the process info corresponding to the given ID.
func (c *Context) Get(id string) (*process.Info, error) {
	logger.Tracef("getting %q from hook context", id)

	actual, ok := c.updates[id]
	if !ok {
		actual, ok = c.processes[id]
		if !ok {
			return nil, errors.NotFoundf("%s", id)
		}
	}
	if actual == nil {
		fetched, err := c.fetch(id)
		if err != nil {
			return nil, errors.Trace(err)
		}
		actual = fetched
	}
	return actual, nil
}

// List returns the sorted names of all registered processes.
func (c *Context) List() ([]string, error) {
	logger.Tracef("listing all procs in hook context")

	procs, err := c.Processes()
	if err != nil {
		return nil, errors.Trace(err)
	}
	if len(procs) == 0 {
		return nil, nil
	}
	var ids []string
	for _, proc := range procs {
		ids = append(ids, proc.ID())
	}
	sort.Strings(ids)
	return ids, nil
}

// Set records the process info in the hook context.
func (c *Context) Set(info process.Info) error {
	logger.Tracef("adding %q to hook context: %#v", info.ID(), info)

	if err := info.Validate(); err != nil {
		return errors.Trace(err)
	}
	// TODO(ericsnow) We are likely missing mechanisim for local persistence.

	c.updates[info.ID()] = &info
	return nil
}

// ListDefinitions returns the unit's charm-defined processes.
func (c *Context) ListDefinitions() ([]charm.Process, error) {
	definitions, err := c.api.AllDefinitions()
	if err != nil {
		return nil, errors.Trace(err)
	}
	return definitions, nil
}

// TODO(ericsnow) The context machinery is not actually using this yet.

// Flush implements jujuc.ContextComponent. In this case that means all
// added and updated process.Info in the hook context are pushed to
// Juju state via the API.
func (c *Context) Flush() error {
	logger.Tracef("flushing from hook context to state")

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
