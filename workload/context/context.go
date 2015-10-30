// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package context

import (
	"sort"

	"github.com/juju/errors"
	"github.com/juju/loggo"

	"github.com/juju/juju/workload"
)

var logger = loggo.GetLogger("juju.payload.context")

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
	Get(class, id string) (*workload.Payload, error)
	// Track records the workload info in the hook context.
	Track(payload workload.Payload) error
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
	payloads map[string]workload.Payload
	updates  map[string]workload.Payload
}

// NewContext returns a new jujuc.ContextComponent for workloads.
func NewContext(api APIClient, dataDir string) *Context {
	return &Context{
		api:      api,
		dataDir:  dataDir,
		payloads: make(map[string]workload.Payload),
		updates:  make(map[string]workload.Payload),
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
		pl := result.Payload
		// TODO(ericsnow) Use id instead of pl.FullID().
		ctx.payloads[pl.FullID()] = pl.Payload
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

// Payloads returns the workloads known to the context.
func (c *Context) Payloads() ([]workload.Payload, error) {
	payloads := mergePayloadMaps(c.payloads, c.updates)
	var newPayloads []workload.Payload
	for _, pl := range payloads {
		newPayloads = append(newPayloads, pl)
	}

	return newPayloads, nil
}

func mergePayloadMaps(payloads, updates map[string]workload.Payload) map[string]workload.Payload {
	// At this point workloads and updates have already been checked for
	// nil values so we won't see any here.
	result := make(map[string]workload.Payload)
	for k, v := range payloads {
		result[k] = v
	}
	for k, v := range updates {
		result[k] = v
	}
	return result
}

// Get returns the workload info corresponding to the given ID.
func (c *Context) Get(class, id string) (*workload.Payload, error) {
	fullID := workload.BuildID(class, id)
	logger.Tracef("getting %q from hook context", fullID)

	actual, ok := c.updates[fullID]
	if !ok {
		actual, ok = c.payloads[fullID]
		if !ok {
			return nil, errors.NotFoundf("%s", fullID)
		}
	}
	return &actual, nil
}

// List returns the sorted names of all registered workloads.
func (c *Context) List() ([]string, error) {
	logger.Tracef("listing all payloads in hook context")

	payloads, err := c.Payloads()
	if err != nil {
		return nil, errors.Trace(err)
	}
	if len(payloads) == 0 {
		return nil, nil
	}
	var ids []string
	for _, wl := range payloads {
		ids = append(ids, wl.FullID())
	}
	sort.Strings(ids)
	return ids, nil
}

// Track records the workload info in the hook context.
func (c *Context) Track(pl workload.Payload) error {
	logger.Tracef("adding %q to hook context: %#v", pl.FullID(), pl)

	if err := pl.Validate(); err != nil {
		return errors.Trace(err)
	}

	// TODO(ericsnow) We are likely missing mechanisim for local persistence.
	id := pl.FullID()
	c.updates[id] = pl
	return nil
}

// Untrack tells juju to stop tracking this workload.
func (c *Context) Untrack(class, id string) error {
	fullID := workload.BuildID(class, id)
	logger.Tracef("Calling untrack on payload context %q", fullID)

	res, err := c.api.Untrack(fullID)
	if err != nil {
		return errors.Trace(err)
	}
	if len(res) > 0 && res[0].Error != nil {
		return errors.Trace(res[0].Error)
	}
	delete(c.payloads, id)

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
// added and updated workload.Payload in the hook context are pushed to
// Juju state via the API.
func (c *Context) Flush() error {
	logger.Tracef("flushing from hook context to state")
	// TODO(natefinch): make this a noop and move this code into set.

	if len(c.updates) > 0 {
		var updates []workload.Payload
		for _, pl := range c.updates {
			updates = append(updates, pl)
		}

		res, err := c.api.Track(updates...)
		if err != nil {
			return errors.Trace(err)
		}
		if len(res) > 0 && res[0].Error != nil {
			return errors.Trace(res[0].Error)
		}

		for k, v := range c.updates {
			c.payloads[k] = v
		}
		c.updates = map[string]workload.Payload{}
	}
	return nil
}
