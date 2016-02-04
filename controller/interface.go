// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package controller

// ControllerWriter writes controller connection data.
type ControllerWriter interface {
	// Write writes the current information to persistent storage.
	Write() error
}
