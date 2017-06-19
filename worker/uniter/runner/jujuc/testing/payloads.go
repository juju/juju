// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	"github.com/juju/juju/apiserver/params"
)

// Payloads holds the values for the hook context.
type Payloads struct {
	trackingPayloads   []params.TrackPayloadParams
	untrackingPayloads []params.UntrackPayloadParams
	statusPayloads     []params.PayloadStatusParams
}

// ContextPaylods is a test double for jujuc.ContextStorage.
type ContextPayloads struct {
	contextBase
	p *Payloads
}

// TrackPayload records the payload in the hook context.
func (c *ContextPayloads) TrackPayload(p params.TrackPayloadParams) error {
	c.stub.AddCall("TrackPayload")
	if c.p.trackingPayloads == nil {
		c.p.trackingPayloads = []params.TrackPayloadParams{}
	}
	c.p.trackingPayloads = append(c.p.trackingPayloads, p)
	return c.stub.NextErr()
}

// UntrackPayload removes the payload from list of payloads to track.
func (c *ContextPayloads) UntrackPayload(p params.UntrackPayloadParams) error {
	c.stub.AddCall("UntrackPayload")
	if c.p.untrackingPayloads == nil {
		c.p.untrackingPayloads = []params.UntrackPayloadParams{}
	}
	c.p.untrackingPayloads = append(c.p.untrackingPayloads, p)
	return c.stub.NextErr()
}

// SetPayloadStatus collects payloads for status change.
func (c *ContextPayloads) SetPayloadStatus(p params.PayloadStatusParams) error {
	c.stub.AddCall("SetPayloadStatus")
	if c.p.statusPayloads == nil {
		c.p.statusPayloads = []params.PayloadStatusParams{}
	}
	c.p.statusPayloads = append(c.p.statusPayloads, p)
	return c.stub.NextErr()
}
