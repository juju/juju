// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package actions

import "time"

// ActionMessage is a timestamped message logged by a running action.
type ActionMessage struct {
	Message   string    `json:"message"`
	Timestamp time.Time `json:"timestamp"`
}
