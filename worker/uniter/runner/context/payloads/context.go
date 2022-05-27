// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package payloads

import (
	"sort"

	"github.com/juju/errors"
	"github.com/juju/loggo"

	corepayloads "github.com/juju/juju/core/payloads"
)

var logger = loggo.GetLogger("juju.payload.context")

// PayloadAPIClient represents the API needs of a PayloadContext.
type PayloadAPIClient interface {
	// List requests the payload info for the given IDs.
	List(fullIDs ...string) ([]corepayloads.Result, error)
	// Track sends a request to update state with the provided payloads.
	Track(payloads ...corepayloads.Payload) ([]corepayloads.Result, error)
	// Untrack removes the payloads from our list track.
	Untrack(fullIDs ...string) ([]corepayloads.Result, error)
	// SetStatus sets the status for the given IDs.
	SetStatus(status string, fullIDs ...string) ([]corepayloads.Result, error)
}

// PayloadsHookContext is the implementation of runner.ContextPayloads.
type PayloadsHookContext struct {
	client PayloadAPIClient

	// TODO(ericsnow) Use the Juju ID for the key rather than Info.ID().
	payloads map[string]corepayloads.Payload
	updates  map[string]corepayloads.Payload
}

// NewContext returns a new hooks.PayloadsHookContext for payloads.
func NewContext(client PayloadAPIClient) (*PayloadsHookContext, error) {
	results, err := client.List()
	if err != nil {
		return nil, errors.Annotate(err, "getting unit payloads")
	}
	ctx := &PayloadsHookContext{
		client:   client,
		payloads: make(map[string]corepayloads.Payload),
		updates:  make(map[string]corepayloads.Payload),
	}
	for _, result := range results {
		pl := result.Payload
		// TODO(ericsnow) Use id instead of pl.FullID().
		ctx.payloads[pl.FullID()] = pl.Payload
	}
	return ctx, nil
}

// Payloads returns the payloads known to the context.
func (c *PayloadsHookContext) Payloads() ([]corepayloads.Payload, error) {
	payloads := mergePayloadMaps(c.payloads, c.updates)
	var keys []string
	for k := range payloads {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var newPayloads []corepayloads.Payload
	for _, k := range keys {
		newPayloads = append(newPayloads, payloads[k])
	}

	return newPayloads, nil
}

func mergePayloadMaps(payloads, updates map[string]corepayloads.Payload) map[string]corepayloads.Payload {
	// At this point payloads and updates have already been checked for
	// nil values so we won't see any here.
	result := make(map[string]corepayloads.Payload)
	for k, v := range payloads {
		result[k] = v
	}
	for k, v := range updates {
		result[k] = v
	}
	return result
}

// GetPayload returns the payload info corresponding to the given ID.
func (c *PayloadsHookContext) GetPayload(class, id string) (*corepayloads.Payload, error) {
	fullID := corepayloads.BuildID(class, id)
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

// ListPayloads returns the sorted names of all registered payloads.
func (c *PayloadsHookContext) ListPayloads() ([]string, error) {
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

// TrackPayload records the payload info in the hook context.
func (c *PayloadsHookContext) TrackPayload(pl corepayloads.Payload) error {
	logger.Tracef("adding %q to hook context: %#v", pl.FullID(), pl)

	if err := pl.Validate(); err != nil {
		return errors.Trace(err)
	}

	// TODO(ericsnow) We are likely missing mechanism for local persistence.
	id := pl.FullID()
	c.updates[id] = pl
	return nil
}

// UntrackPayload tells juju to stop tracking this payload.
func (c *PayloadsHookContext) UntrackPayload(class, id string) error {
	fullID := corepayloads.BuildID(class, id)
	logger.Tracef("Calling untrack on payload context %q", fullID)

	res, err := c.client.Untrack(fullID)
	if err != nil {
		return errors.Trace(err)
	}
	// TODO(ericsnow) We should not ignore a 0-len result.
	if len(res) > 0 && res[0].Error != nil {
		return errors.Trace(res[0].Error)
	}
	delete(c.payloads, fullID)

	return nil
}

// SetPayloadStatus sets the identified payload's status.
func (c *PayloadsHookContext) SetPayloadStatus(class, id, status string) error {
	fullID := corepayloads.BuildID(class, id)
	logger.Tracef("Calling status-set on payload context %q", fullID)

	res, err := c.client.SetStatus(status, fullID)
	if err != nil {
		return errors.Trace(err)
	}
	// TODO(ericsnow) We should not ignore a 0-len result.
	if len(res) > 0 && res[0].Error != nil {
		// In a hook context, the case where the specified payload does
		// not exist is a special one. A hook tool is how a charm author
		// communicates the state of the charm. So returning an error
		// here in the "missing" case makes less sense than in other
		// places. We could simply ignore any error that surfaces for
		// that case. However, returning the error communicates to the
		// charm author that what they're trying to communicate doesn't
		// make sense.
		return errors.Trace(res[0].Error)
	}

	return nil
}

// TODO(ericsnow) The context machinery is not actually using this yet.

// FlushPayloads implements context.ContextPayloads. In this case that means all
// added and updated payloads.Payload in the hook context are pushed to
// Juju state via the API.
func (c *PayloadsHookContext) FlushPayloads() error {
	logger.Tracef("flushing from hook context to state")
	// TODO(natefinch): make this a noop and move this code into set.

	if len(c.updates) > 0 {
		var updates []corepayloads.Payload
		for _, pl := range c.updates {
			updates = append(updates, pl)
		}

		res, err := c.client.Track(updates...)
		if err != nil {
			return errors.Trace(err)
		}
		if len(res) > 0 && res[0].Error != nil {
			return errors.Trace(res[0].Error)
		}

		for k, v := range c.updates {
			c.payloads[k] = v
		}
		c.updates = map[string]corepayloads.Payload{}
	}
	return nil
}
