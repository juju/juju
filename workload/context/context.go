// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package context

import (
	"sort"

	"github.com/juju/errors"
	"github.com/juju/loggo"

	"github.com/juju/juju/workload"
)

var logger = loggo.GetLogger("juju.workload.context")

// APIClient represents the API needs of a Context.
type APIClient interface {
	// List requests the workload info for the given IDs.
	List(fullIDs ...string) ([]workload.Result, error)
	// Register sends a request to update state with the provided workloads.
	Track(payloads ...workload.Payload) ([]workload.Result, error)
	// Untrack removes the workloads from our list track.
	Untrack(fullIDs ...string) ([]workload.Result, error)
	// SetStatus sets the status for the given IDs.
	SetStatus(status string, fullIDs ...string) ([]workload.Result, error)
}

// TODO(ericsnow) Rename Get and Set to more specifically describe what
// they are for.

// Component provides the hook context data specific to workloads.
type Component interface {
	// Get returns the workload info corresponding to the given ID.
	Get(class, id string) (*workload.Info, error)
	// Track records the workload info in the hook context.
	Track(info workload.Info) error
	// Untrack removes the workload from our list of workloads to track.
	Untrack(class, id string) error
	// SetStatus sets the status of the payload.
	SetStatus(class, id, status string) error
	// List returns the list of registered workload IDs.
	List() ([]string, error)
	// Flush pushes the hook context data out to state.
	Flush() error
}

var _ Component = (*Context)(nil)

// Context is the workload portion of the hook context.
type Context struct {
	api     APIClient
	dataDir string
	// TODO(ericsnow) Use the Juju ID for the key rather than Info.ID().
	workloads map[string]workload.Info
	updates   map[string]workload.Info
}

// NewContext returns a new jujuc.ContextComponent for workloads.
func NewContext(api APIClient, dataDir string) *Context {
	return &Context{
		api:       api,
		dataDir:   dataDir,
		workloads: make(map[string]workload.Info),
		updates:   make(map[string]workload.Info),
	}
}

// NewContextAPI returns a new jujuc.ContextComponent for workloads.
func NewContextAPI(api APIClient, dataDir string) (*Context, error) {
	results, err := api.List()
	if err != nil {
		return nil, errors.Trace(err)
	}

	ctx := NewContext(api, dataDir)
	for _, result := range results {
		wl := result.Workload
		// TODO(ericsnow) Use id instead of wl.ID().
		ctx.workloads[wl.ID()] = *wl
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
func (c *Context) Get(class, id string) (*workload.Info, error) {
	fullID := workload.BuildID(class, id)
	logger.Tracef("getting %q from hook context", fullID)

	actual, ok := c.updates[fullID]
	if !ok {
		actual, ok = c.workloads[fullID]
		if !ok {
			return nil, errors.NotFoundf("%s", fullID)
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

// Track records the workload info in the hook context.
func (c *Context) Track(info workload.Info) error {
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
func (c *Context) Untrack(class, id string) error {
	fullID := workload.BuildID(class, id)
	logger.Tracef("Calling untrack on workload context %q", fullID)

	res, err := c.api.Untrack(fullID)
	if err != nil {
		return errors.Trace(err)
	}
	if len(res) > 0 && res[0].Error != nil {
		return errors.Trace(res[0].Error)
	}
	delete(c.workloads, id)

	return nil
}

func (c *Context) SetStatus(class, id, status string) error {
	fullID := workload.BuildID(class, id)
	logger.Tracef("Calling status-set on payload context %q", fullID)

	res, err := c.api.SetStatus(status, fullID)
	if err != nil {
		return errors.Trace(err)
	}
	if len(res) > 0 && res[0].Error != nil {
		return errors.Trace(res[0].Error)
	}

	return nil
}

// TODO(ericsnow) The context machinery is not actually using this yet.

// Flush implements jujuc.ContextComponent. In this case that means all
// added and updated workload.Info in the hook context are pushed to
// Juju state via the API.
func (c *Context) Flush() error {
	logger.Tracef("flushing from hook context to state")
	// TODO(natefinch): make this a noop and move this code into set.

	if len(c.updates) > 0 {
		var updates []workload.Payload
		for _, info := range c.updates {
			updates = append(updates, info.AsPayload())
		}

		res, err := c.api.Track(updates...)
		if err != nil {
			return errors.Trace(err)
		}
		if len(res) > 0 && res[0].Error != nil {
			return errors.Trace(res[0].Error)
		}

		for k, v := range c.updates {
			c.workloads[k] = v
		}
		c.updates = map[string]workload.Info{}
	}
	return nil
}
