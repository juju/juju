// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package jujuctesting

import "github.com/juju/juju/core/payloads"

// ContextPayloads is a test double for jujuc.ContextResources.
type ContextPayloads struct {
	contextBase
}

// GetPayload implements jujuc.ContextPayloads.
func (c *ContextPayloads) GetPayload(class, id string) (*payloads.Payload, error) {
	c.stub.AddCall("GetPayload", class, id)
	return &payloads.Payload{}, nil
}

// TrackPayload implements jujuc.ContextPayloads.
func (c *ContextPayloads) TrackPayload(payload payloads.Payload) error {
	c.stub.AddCall("TrackPayload", payload)
	return nil
}

// UntrackPayload implements jujuc.ContextPayloads.
func (c *ContextPayloads) UntrackPayload(class, id string) error {
	c.stub.AddCall("UntrackPayload", class, id)
	return nil
}

// SetPayloadStatus implements jujuc.ContextPayloads.
func (c *ContextPayloads) SetPayloadStatus(class, id, status string) error {
	c.stub.AddCall("SetPayloadStatus", class, id, status)
	return nil
}

// ListPayloads implements jujuc.ContextPayloads.
func (c *ContextPayloads) ListPayloads() ([]string, error) {
	c.stub.AddCall("ListPayloads")
	return nil, nil
}

// FlushPayloads implements jujuc.ContextPayloads.
func (c *ContextPayloads) FlushPayloads() error {
	c.stub.AddCall("FlushPayloads")
	return nil
}
