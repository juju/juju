// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package statushistory

import (
	"context"
	"time"

	"github.com/juju/clock"

	"github.com/juju/juju/core/status"
)

// Record represents a single record of status information.
type Record struct {
	Name    string
	ID      string
	Message string
	Status  string
	Time    string
	Data    map[string]any
}

// Recorder is an interface for recording status information.
type Recorder interface {
	// Record records the given status information.
	Record(context.Context, Record) error
}

// Namespace represents a namespace of the status we're recording.
type Namespace struct {
	Name string
	ID   string
}

// WithID returns a new namespace with the given ID.
func (n Namespace) WithID(id string) Namespace {
	return Namespace{
		Name: n.Name,
		ID:   id,
	}
}

func (n Namespace) String() string {
	if n.ID == "" {
		return n.Name
	}
	return n.Name + " (" + n.ID + ")"
}

// StatusHistory records status information into a generalized way.
type StatusHistory struct {
	recorder Recorder
	clock    clock.Clock
}

// NewStatusHistory creates a new StatusHistory.
func NewStatusHistory(recorder Recorder, clock clock.Clock) *StatusHistory {
	return &StatusHistory{
		recorder: recorder,
		clock:    clock,
	}
}

// RecordStatus records the given status information.
// If the status data cannot be marshalled, it will not be recorded, instead
// the error will be logged under the data_error key.
func (s *StatusHistory) RecordStatus(ctx context.Context, ns Namespace, status status.StatusInfo) error {
	var now time.Time
	if since := status.Since; since != nil && !since.IsZero() {
		now = *since
	} else {
		now = s.clock.Now()
	}

	return s.recorder.Record(ctx, Record{
		Name:    ns.Name,
		ID:      ns.ID,
		Message: status.Message,
		Status:  status.Status.String(),
		Time:    now.Format(time.RFC3339),
		Data:    status.Data,
	})
}
