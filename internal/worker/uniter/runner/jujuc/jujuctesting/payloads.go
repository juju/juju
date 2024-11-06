// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package jujuctesting

import (
	"context"

	"github.com/juju/juju/core/payloads"
)

// ContextPayloads is a test double for jujuc.ContextResources.
type ContextPayloads struct {
	contextBase
}

// GetPayload implements jujuc.ContextPayloads.
func (c *ContextPayloads) GetPayload(ctx context.Context, class, id string) (*payloads.Payload, error) {
	c.stub.AddCall("GetPayload", class, id)
	return &payloads.Payload{}, nil
}

// TrackPayload implements jujuc.ContextPayloads.
func (c *ContextPayloads) TrackPayload(_ context.Context, payload payloads.Payload) error {
	c.stub.AddCall("TrackPayload", payload)
	return nil
}

// UntrackPayload implements jujuc.ContextPayloads.
func (c *ContextPayloads) UntrackPayload(_ context.Context, class, id string) error {
	c.stub.AddCall("UntrackPayload", class, id)
	return nil
}

// SetPayloadStatus implements jujuc.ContextPayloads.
func (c *ContextPayloads) SetPayloadStatus(_ context.Context, class, id, status string) error {
	c.stub.AddCall("SetPayloadStatus", class, id, status)
	return nil
}

// ListPayloads implements jujuc.ContextPayloads.
func (c *ContextPayloads) ListPayloads(_ context.Context) ([]string, error) {
	c.stub.AddCall("ListPayloads")
	return nil, nil
}

// FlushPayloads implements jujuc.ContextPayloads.
func (c *ContextPayloads) FlushPayloads(_ context.Context) error {
	c.stub.AddCall("FlushPayloads")
	return nil
}
