// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package context

import (
	"os"
	"path/filepath"
	"sort"

	"github.com/juju/errors"
	"github.com/juju/loggo"

	"github.com/juju/juju/workload"
	"github.com/juju/juju/workload/plugin"
)

var logger = loggo.GetLogger("juju.workload.context")

// APIClient represents the API needs of a Context.
type APIClient interface {
	// List requests the workload info for the given IDs.
	List(ids ...string) ([]workload.Info, error)
	// Register sends a request to update state with the provided workloads.
	Track(workloads ...workload.Info) ([]string, error)
	// Untrack removes the workloads from our list track.
	Untrack(ids []string) ([]workload.Result, error)
	// SetStatus sets the status for the given IDs.
	SetStatus(status workload.Status, pluginStatus workload.PluginStatus, ids ...string) error
}

// TODO(ericsnow) Rename Get and Set to more specifically describe what
// they are for.

// Component provides the hook context data specific to workloads.
type Component interface {
	// Plugin returns the plugin to use for the given workload.
	Plugin(info *workload.Info, path string) (workload.Plugin, error)
	// Get returns the workload info corresponding to the given ID.
	Get(id string) (*workload.Info, error)
	// Track records the workload info in the hook context.
	Track(info workload.Info) error
	// Untrack removes the workload from our list of workloads to track.
	Untrack(id string) error
	// List returns the list of registered workload IDs.
	List() ([]string, error)
	// Flush pushes the hook context data out to state.
	Flush() error
}

var _ Component = (*Context)(nil)

// Context is the workload portion of the hook context.
type Context struct {
	api       APIClient
	dataDir   string
	plugin    workload.Plugin
	workloads map[string]workload.Info
	updates   map[string]workload.Info

	addEvents func(...workload.Event) error
	// FindPlugin is the function used to find the plugin for the given
	// plugin name.
	FindPlugin func(pluginName, dataDir string) (workload.Plugin, error)
}

// NewContext returns a new jujuc.ContextComponent for workloads.
func NewContext(api APIClient, dataDir string, addEvents func(...workload.Event) error) *Context {
	return &Context{
		api:        api,
		dataDir:    dataDir,
		workloads:  make(map[string]workload.Info),
		updates:    make(map[string]workload.Info),
		addEvents:  addEvents,
		FindPlugin: plugin.Find,
	}
}

// NewContextAPI returns a new jujuc.ContextComponent for workloads.
func NewContextAPI(api APIClient, dataDir string, addEvents func(...workload.Event) error) (*Context, error) {
	workloads, err := api.List()
	if err != nil {
		return nil, errors.Trace(err)
	}

	ctx := NewContext(api, dataDir, addEvents)
	for _, wl := range workloads {
		ctx.workloads[wl.ID()] = wl
	}
	return ctx, nil
}

// HookContext is the portion of jujuc.Context used in this package.
type HookContext interface {
	// Component implements jujuc.Context.
	Component(string) (Component, error)
}

// ContextComponent returns the hook context for the workload
// workload component.
func ContextComponent(ctx HookContext) (Component, error) {
	compCtx, err := ctx.Component(workload.ComponentName)
	if errors.IsNotFound(err) {
		return nil, errors.Errorf("component %q not registered", workload.ComponentName)
	}
	if err != nil {
		return nil, errors.Trace(err)
	}
	if compCtx == nil {
		return nil, errors.Errorf("component %q disabled", workload.ComponentName)
	}
	return compCtx, nil
}

// Plugin returns the plugin to use in this context.
func (c *Context) Plugin(info *workload.Info, envPath string) (workload.Plugin, error) {
	if c.plugin != nil {
		return c.plugin, nil
	}

	if envPath != "" {
		envPath += string(filepath.ListSeparator) + os.Getenv("PATH")
		if err := os.Setenv("PATH", envPath); err != nil {
			return nil, errors.Trace(err)
		}
	}

	plugin, err := c.FindPlugin(info.Type, c.dataDir)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return plugin, nil
}

// TODO(ericsnow) Should we build in refreshes in all the methods?

// Workloads returns the workloads known to the context.
func (c *Context) Workloads() ([]workload.Info, error) {
	workloads := mergeWorkloadMaps(c.workloads, c.updates)
	var newWorkloads []workload.Info
	for _, info := range workloads {
		newWorkloads = append(newWorkloads, info)
	}

	return newWorkloads, nil
}

func mergeWorkloadMaps(workloads, updates map[string]workload.Info) map[string]workload.Info {
	// At this point workloads and updates have already been checked for
	// nil values so we won't see any here.
	result := make(map[string]workload.Info)
	for k, v := range workloads {
		result[k] = v
	}
	for k, v := range updates {
		result[k] = v
	}
	return result
}

// Get returns the workload info corresponding to the given ID.
func (c *Context) Get(id string) (*workload.Info, error) {
	logger.Tracef("getting %q from hook context", id)

	actual, ok := c.updates[id]
	if !ok {
		actual, ok = c.workloads[id]
		if !ok {
			return nil, errors.NotFoundf("%s", id)
		}
	}
	return &actual, nil
}

// List returns the sorted names of all registered workloads.
func (c *Context) List() ([]string, error) {
	logger.Tracef("listing all workloads in hook context")

	workloads, err := c.Workloads()
	if err != nil {
		return nil, errors.Trace(err)
	}
	if len(workloads) == 0 {
		return nil, nil
	}
	var ids []string
	for _, wl := range workloads {
		ids = append(ids, wl.ID())
	}
	sort.Strings(ids)
	return ids, nil
}

// TODO(ericsnow) rename to Track

// Track records the workload info in the hook context.
func (c *Context) Track(info workload.Info) error {
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

// Untrack tells juju to stop tracking this workload.
func (c *Context) Untrack(id string) error {
	logger.Tracef("Calling untrack on workload context %q", id)

	res, err := c.api.Untrack([]string{id})
	if err != nil {
		return errors.Trace(err)
	}
	if len(res) > 0 && res[0].Err != nil {
		return errors.Trace(res[0].Err)
	}
	delete(c.workloads, id)

	return nil
}

// TODO(ericsnow) The context machinery is not actually using this yet.

// Flush implements jujuc.ContextComponent. In this case that means all
// added and updated workload.Info in the hook context are pushed to
// Juju state via the API.
func (c *Context) Flush() error {
	logger.Tracef("flushing from hook context to state")
	// TODO(natefinch): make this a noop and move this code into set.

	var events []workload.Event
	if len(c.updates) > 0 {
		var updates []workload.Info
		for _, info := range c.updates {
			updates = append(updates, info)

			plugin, err := c.Plugin(&info, "")
			if err != nil {
				return errors.Trace(err)
			}
			events = append(events, workload.Event{
				Kind:     workload.EventKindTracked,
				ID:       info.ID(),
				Plugin:   plugin,
				PluginID: info.Details.ID,
			})
		}
		if _, err := c.api.Track(updates...); err != nil {
			return errors.Trace(err)
		}
		for k, v := range c.updates {
			c.workloads[k] = v
		}
		c.updates = map[string]workload.Info{}
	}
	if len(events) > 0 {
		err := c.addEvents(events...)
		if errors.Cause(err) == workload.EventsClosed {
			logger.Infof("ignoring closed workload events")
		} else if err != nil {
			return errors.Trace(err)
		}
	}
	return nil
}
