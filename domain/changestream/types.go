// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package changestream

import (
	"fmt"
	"time"
)

// Window represents a time window with a start and end time.
type Window struct {
	Start, End time.Time
}

// Contains returns true if the Window contains the given time.
func (w Window) Contains(o Window) bool {
	if w.Equals(o) {
		return true
	}
	return w.Start.Before(o.Start) && w.End.After(o.End)
}

// Equals returns true if the Window is equal to the given Window.
func (w Window) Equals(o Window) bool {
	return w.Start.Equal(o.Start) && w.End.Equal(o.End)
}

func (w Window) String() string {
	return fmt.Sprintf("start: %s, end: %s", w.Start.Format(time.RFC3339), w.End.Format(time.RFC3339))
}
