// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package operation

import "github.com/juju/juju/internal/uuid"

// Action represents a domain action.
type Action struct {
	// UUID is the action unique identifier.
	UUID uuid.UUID
	// Receiver is the action receiver (unit / machine).
	Receiver string
}

// CompletedTaskResult holds the task ID and output used when recording
// the result of an task.
type CompletedTaskResult struct {
	TaskID  string
	Status  string
	Results map[string]interface{}
	Message string
}
