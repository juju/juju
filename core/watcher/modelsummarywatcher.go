// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package watcher

import (
	"time"
)

// Status values show a high level indicator of model health.
const (
	StatusRed    = "red"
	StatusYellow = "yellow" // amber?
	StatusGreen  = "green"
)

// ModelSummary represents a high level view of a model. Primary use case is to
// drive a dashboard with model level overview.
type ModelSummary struct {
	UUID string
	// Removed indicates that the user can no longer see this model, because it has
	// either been removed, or there access revoked.
	Removed bool

	Controller  string
	Namespace   string
	Name        string
	Admins      []string
	Status      string
	Annotations map[string]string

	// Messages contain status message for any unit status in error.
	Messages []ModelSummaryMessage

	Cloud        string
	Region       string
	Credential   string
	LastModified time.Time // Currently missing in cache and underlying db model.

	// MachineCount is just top level machines.
	MachineCount     int
	ContainerCount   int
	ApplicationCount int
	UnitCount        int
	RelationCount    int
}

// ModelSummaryMessage holds information about an error message from an
// agent, and when that message was set.
type ModelSummaryMessage struct {
	Agent   string
	Message string
}

// ModelSummaryWatcher will return a slice for all existing models
type ModelSummaryWatcher interface {
	CoreWatcher
	Changes() <-chan []ModelSummary
}
