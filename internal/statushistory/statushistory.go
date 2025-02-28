// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package statushistory

import (
	"context"
	"encoding/json"
	"time"

	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/status"
)

// Recorder is an interface for recording status information.
type Recorder interface {
	// Logf logs information at the given level.
	// The provided arguments are assembled together into a string with
	// fmt.Sprintf.
	Logf(ctx context.Context, level logger.Level, labels logger.Labels, format string, args ...any)
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
	return n.Name + " (" + n.ID + ")"
}

// StatusHistory records status information into a generalized way.
type StatusHistory struct {
	logger logger.Logger
}

// NewStatusHistory creates a new StatusHistory.
func NewStatusHistory(logger logger.Logger) *StatusHistory {
	return &StatusHistory{
		logger: logger,
	}
}

// RecordStatus records the given status information.
// If the status data cannot be marshalled, it will not be recorded, instead
// the error will be logged under the data_error key.
func (s *StatusHistory) RecordStatus(ctx context.Context, ns Namespace, status status.StatusInfo) {
	labels := logger.Labels{
		namespaceNameKey: ns.Name,
		statusKey:        status.Status.String(),
		messageKey:       status.Message,
		sinceKey:         status.Since.Format(time.RFC3339),
	}

	// Only include the namespace ID if it's set.
	if ns.ID != "" {
		labels[namespaceIDKey] = ns.ID
	}

	// For structured logging this is less than ideal, as we'll have JSON
	// encoded inside of JSON. However, it's the best we can do without
	// an alternative.
	data, err := json.Marshal(status.Data)
	if err != nil {
		labels[dataErrorKey] = err.Error()
	} else {
		labels[dataKey] = string(data)
	}

	s.logger.Logf(ctx, logger.INFO, labels, "status: %s", status.Message)
}

const (
	namespaceNameKey = "namespace_name"
	namespaceIDKey   = "namespace_id"
	statusKey        = "status"
	messageKey       = "message"
	sinceKey         = "since"
	dataKey          = "data"
	dataErrorKey     = "data_error"
)
