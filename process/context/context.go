// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package context

import (
	"sort"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"gopkg.in/juju/charm.v5"

	"github.com/juju/juju/process"
	"github.com/juju/juju/process/plugin"
)

var logger = loggo.GetLogger("juju.process.context")

// TODO(ericsnow) Normalize method names across all the arch. layers
// (e.g. List->AllProcesses, Get->Processes, Set->AddProcess).

// APIClient represents the API needs of a Context.
type APIClient interface {
	// ListProcesses requests the process info for the given ID.
	ListProcesses(ids ...string) ([]process.Info, error)
	// RegisterProcesses sends a request to update state with the provided processes.
	RegisterProcesses(procs ...process.Info) ([]string, error)
	// Untrack removes the processes from our list of processes to track.
	Untrack(ids []string) error
	// AllDefinitions returns the process definitions found in the
	// unit's metadata.
	AllDefinitions() ([]charm.Process, error)
	// SetProcessesStatus sets the status for the given ID.
	SetProcessesStatus(status process.Status, pluginStatus process.PluginStatus, ids ...string) error
}

// TODO(ericsnow) Rename Get and Set to more specifically describe what
// they are for.

// Component provides the hook context data specific to workload processes.
type Component interface {
	// Plugin returns the plugin to use for the given proc.
	Plugin(info *process.Info) (process.Plugin, error)
	// Get returns the process info corresponding to the given ID.
	Get(id string) (*process.Info, error)
	// Set records the process info in the hook context.
	Set(info process.Info) error
	// Untrack removes the process from our list of processes to track.
	Untrack(id string)
	// List returns the list of registered process IDs.
	List() ([]string, error)
	// ListDefinitions returns the charm-defined processes.
	ListDefinitions() ([]charm.Process, error)
	// Flush pushes the hook context data out to state.
	Flush() error
}

var _ Component = (*Context)(nil)

// Context is the workload process portion of the hook context.
type Context struct {
	api       APIClient
	plugin    process.Plugin
	processes map[string]process.Info
	updates   map[string]process.Info
	removes   map[string]struct{}

	addEvents func(...process.Event)
	// FindPlugin is the function used to find the plugin for the given
	// plugin name.
	FindPlugin func(pluginName string) (process.Plugin, error)
}

// NewContext returns a new jujuc.ContextComponent for workload processes.
func NewContext(api APIClient, addEvents func(...process.Event)) *Context {
	return &Context{
		api:       api,
		processes: make(map[string]process.Info),
		updates:   make(map[string]process.Info),
		removes:   make(map[string]struct{}),
		addEvents: addEvents,
		FindPlugin: func(ptype string) (process.Plugin, error) {
			return plugin.Find(ptype)
		},
	}
}

// NewContextAPI returns a new jujuc.ContextComponent for workload processes.
func NewContextAPI(api APIClient, addEvents func(...process.Event)) (*Context, error) {
	procs, err := api.ListProcesses()
	if err != nil {
		return nil, errors.Trace(err)
	}

	ctx := NewContext(api, addEvents)
	for _, proc := range procs {
		ctx.processes[proc.ID()] = proc
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

// Plugin returns the plugin to use in this context.
func (c *Context) Plugin(info *process.Info) (process.Plugin, error) {
	if c.plugin != nil {
		return c.plugin, nil
	}

	plugin, err := c.FindPlugin(info.Type)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return plugin, nil
}

// TODO(ericsnow) Should we build in refreshes in all the methods?

// Processes returns the processes known to the context.
func (c *Context) Processes() ([]process.Info, error) {
	processes := mergeProcMaps(c.processes, c.updates)
	var procs []process.Info
	for _, info := range processes {
		procs = append(procs, info)
	}

	return procs, nil
}

func mergeProcMaps(procs, updates map[string]process.Info) map[string]process.Info {
	// At this point procs and updates have already been checked for
	// nil values so we won't see any here.
	result := make(map[string]process.Info)
	for k, v := range procs {
		result[k] = v
	}
	for k, v := range updates {
		result[k] = v
	}
	return result
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
	return &actual, nil
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

// TODO(ericsnow) rename to Track

// Set records the process info in the hook context.
func (c *Context) Set(info process.Info) error {
	// TODO(ericsnow) rename to Track
	logger.Tracef("adding %q to hook context: %#v", info.ID(), info)

	if err := info.Validate(); err != nil {
		return errors.Trace(err)
	}
	// TODO(ericsnow) We are likely missing mechanisim for local persistence.
	id := info.ID()
	c.updates[id] = info
	return nil
}

// Untrack tells juju to stop tracking this process.
func (c *Context) Untrack(id string) {
	// We assume that flush always gets called immediately after a set/untrack,
	// so we don't have to worry about conflicting updates/deletes.
	c.removes[id] = struct{}{}
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

	// TODO(natefinch): make this a noop and move this code into set/untrack.

	var events []process.Event
	if len(c.updates) > 0 {
		var updates []process.Info
		for _, info := range c.updates {
			updates = append(updates, info)

			plugin, err := c.Plugin(&info)
			if err != nil {
				return errors.Trace(err)
			}
			events = append(events, process.Event{
				Kind:     process.EventKindTracked,
				ID:       info.ID(),
				Plugin:   plugin,
				PluginID: info.Details.ID,
			})
		}
		if _, err := c.api.RegisterProcesses(updates...); err != nil {
			return errors.Trace(err)
		}
		for k, v := range c.updates {
			c.processes[k] = v
		}
		c.updates = map[string]process.Info{}
	}
	if len(c.removes) > 0 {
		removes := make([]string, 0, len(c.removes))
		for id := range c.removes {
			info := c.processes[id]
			removes = append(removes, id)
			delete(c.processes, id)

			plugin, err := c.Plugin(&info)
			if err != nil {
				return errors.Trace(err)
			}
			events = append(events, process.Event{
				Kind:     process.EventKindUntracked,
				ID:       id,
				Plugin:   plugin,
				PluginID: info.Details.ID,
			})
		}
		c.api.Untrack(removes)
		c.removes = map[string]struct{}{}
	}
	if len(events) > 0 {
		c.addEvents(events...)
	}

	return nil
}
