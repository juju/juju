// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package statushistory

import (
	"time"

	"github.com/juju/juju/core/status"
)

// History represents the status history.
type History struct {
	Timestamp time.Time
	Kind      status.HistoryKind
	Status    status.Status
	Message   string
}

// ReadOnlyModel represents a read-only model for the status history.
type ReadOnlyModel struct {
	UUID  string
	Name  string
	Owner string
}
