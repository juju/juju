// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package statushistory

import "github.com/juju/juju/core/status"

// History represents the status history.
type History struct {
	Kind status.HistoryKind
}

// ReadOnlyModel represents a read-only model for the status history.
type ReadOnlyModel struct {
	UUID  string
	Name  string
	Owner string
}
